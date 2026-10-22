# Offline Validation Quickstart

1. Install Go and Node versions documented in the root README.
2. Run `make test` and `make build` from the repository root.
3. Start `./bin/spectide serve --host 127.0.0.1 --port 4747`.
4. In another terminal run `./bin/spectide demo --project ./examples/fake-provider-demo`.
5. Open `http://127.0.0.1:4747`, select the created run, and verify every stage says `SIMULATED`/fake where appropriate.
6. Run `./bin/spectide report <run-id>` and verify required simulated quality gates are present.

Live provider checks require externally authenticated authorized profiles and must remain `NOT_RUN` if unavailable.
