package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
)

var requiredGateInventory = []string{"analyze_clean", "required_tests", "converged", "final_review", "simulated_e2e", "workflow_execution"}

// executionFromConfiguration projects the execution class from the immutable
// routing snapshot so run history is accurate before a selected report loads.
func executionFromConfiguration(encoded string) string {
	var snapshot struct {
		Stages []struct {
			Decision struct {
				Route struct {
					Provider string `json:"provider"`
				} `json:"route"`
			} `json:"decision"`
		} `json:"stages"`
	}
	if encoded == "" || json.Unmarshal([]byte(encoded), &snapshot) != nil || len(snapshot.Stages) == 0 {
		return "UNKNOWN"
	}
	hasLive, hasSimulated := false, false
	for _, stage := range snapshot.Stages {
		if stage.Decision.Route.Provider == "fake" {
			hasSimulated = true
		} else if stage.Decision.Route.Provider != "" {
			hasLive = true
		}
	}
	if hasLive && hasSimulated {
		return "MIXED"
	}
	if hasLive {
		return "LIVE"
	}
	if hasSimulated {
		return "SIMULATED"
	}
	return "UNKNOWN"
}

func executionFromRouteEvidence(encoded, routingProviders, stageProviders string) string {
	if execution := executionFromConfiguration(encoded); execution != "UNKNOWN" {
		return execution
	}
	hasLive, hasSimulated := false, false
	for _, provider := range strings.Split(routingProviders+","+stageProviders, ",") {
		if provider == "fake" {
			hasSimulated = true
		} else if provider != "" {
			hasLive = true
		}
	}
	if hasLive && hasSimulated {
		return "MIXED"
	}
	if hasLive {
		return "LIVE"
	}
	if hasSimulated {
		return "SIMULATED"
	}
	return "UNKNOWN"
}

func initializeGateInventoryTx(ctx context.Context, tx *sql.Tx, runID string) error {
	for _, key := range requiredGateInventory {
		evidence := "required gate has not run"
		if key == "workflow_execution" {
			evidence = "workflow has not reached a terminal outcome"
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO quality_gates(run_id,gate_key,verdict,evidence) VALUES(?,?,?,?)`, runID, key, "NOT_RUN", evidence); err != nil {
			return err
		}
	}
	return appendEvent(ctx, tx, runID, "", "quality.inventory_initialized", "info", map[string]any{"gate_keys": requiredGateInventory, "initial_verdict": "NOT_RUN"})
}

func finalizeTerminalGatesTx(ctx context.Context, tx *sql.Tx, runID string, state model.RunState) error {
	verdict, evidence := "BLOCKED", "workflow stopped before all required work completed"
	switch state {
	case model.Completed:
		verdict, evidence = "PASS", "workflow reached a successful terminal outcome"
	case model.Failed:
		verdict, evidence = "FAIL", "workflow terminated after an execution failure"
	case model.Blocked:
		verdict, evidence = "BLOCKED", "workflow terminated with blocked or unavailable work"
	case model.Cancelled:
		verdict, evidence = "BLOCKED", "workflow was cancelled before all required work completed"
	default:
		return fmt.Errorf("TERMINAL_GATE_STATE_INVALID")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO quality_gates(run_id,gate_key,verdict,evidence) VALUES(?,?,?,?) ON CONFLICT(run_id,gate_key) DO UPDATE SET verdict=excluded.verdict,evidence=excluded.evidence`, runID, "workflow_execution", verdict, evidence); err != nil {
		return err
	}
	decisions, err := acceptanceDecisionsTx(ctx, tx, runID)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT gate_key,verdict,evidence FROM quality_gates WHERE run_id=? ORDER BY gate_key`, runID)
	if err != nil {
		return err
	}
	var gates []model.Gate
	for rows.Next() {
		var gate model.Gate
		if err = rows.Scan(&gate.Key, &gate.Verdict, &gate.Evidence); err != nil {
			rows.Close()
			return err
		}
		gates = append(gates, gate)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	acceptance := model.SummarizeAcceptance(decisions, gates)
	return appendEvent(ctx, tx, runID, "", "run.acceptance", "info", map[string]any{"acceptance": acceptance, "terminal_state": state})
}

type acceptanceQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func acceptanceDecisionsTx(ctx context.Context, tx acceptanceQueryer, runID string) ([]model.RoutingDecision, error) {
	var stored []model.RoutingDecision
	rows, err := tx.QueryContext(ctx, `SELECT stage,provider,profile,model,effort,policy,explanation FROM routing_decisions WHERE run_id=? ORDER BY stage`, runID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item model.RoutingDecision
		if err = rows.Scan(&item.Stage, &item.Provider, &item.Profile, &item.Model, &item.Effort, &item.Policy, &item.Explanation); err != nil {
			rows.Close()
			return nil, err
		}
		stored = append(stored, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	var encoded string
	err = tx.QueryRowContext(ctx, `SELECT config_json FROM run_configurations WHERE run_id=?`, runID).Scan(&encoded)
	if err == sql.ErrNoRows {
		return stored, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot struct {
		Stages []struct {
			Stage    string `json:"stage"`
			Decision struct {
				Stage       string       `json:"stage"`
				Route       config.Route `json:"route"`
				Mode        string       `json:"mode"`
				Explanation string       `json:"explanation"`
			} `json:"decision"`
		} `json:"stages"`
	}
	if err = json.Unmarshal([]byte(encoded), &snapshot); err != nil || len(snapshot.Stages) == 0 {
		return stored, nil
	}
	decisions := make([]model.RoutingDecision, 0, len(snapshot.Stages))
	for _, stage := range snapshot.Stages {
		name := stage.Decision.Stage
		if name == "" {
			name = stage.Stage
		}
		route := stage.Decision.Route
		decisions = append(decisions, model.RoutingDecision{Stage: name, Provider: route.Provider, Profile: route.Profile, Model: route.Model, Effort: route.Effort, Policy: stage.Decision.Mode, Explanation: stage.Decision.Explanation})
	}
	return decisions, nil
}
