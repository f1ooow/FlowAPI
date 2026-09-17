# Verification record

## Parent verification during resumed implementation

- PASS: `go test ./controller ./relay/... ./service ./model ./middleware ./dto ./common ./setting/...`.
- PASS: `cd relaykit && GOWORK=off go build ./...`.
- PASS: `/private/tmp/flowapi-hedge-browser-static-qa.mjs` against the local
  production build and mocked APIs: timeout load/clear/save/reopen, mobile
  bounds, global loser billing default true, false save/reload, keyboard true
  save, desktop/mobile inspection. No production API calls.
- PASS: independent focused model statistics and ClickHouse-pagination tests
  using SQLite fixtures. Live external database services were not exercised.
- PASS: first frontend scope — 39 focused tests, typecheck, changed-file lint,
  production build, zhCN i18n sync. Later `not_billed` UI work requires its own
  follow-up verification before final acceptance.

## Final verification after review fixes

- PASS: full affected backend suite rerun on final implementation:
  `go test ./controller ./relay/... ./service ./model ./middleware ./dto ./common ./setting/...`.
- PASS: targeted real-upstream lifecycle/billing/resource race tests:
  `go test -race ./controller ./relay/helper ./relay/common ./relay/channel ./service ./model ./common ./setting/operation_setting -run 'TestHedge|TestNonStreamDeadline|TestForkBodyStorage|TestBillHedge|TestSumUsedQuota' -count=1`.
  The `relay/common` package has no test names matching this filter; its full
  race run passed separately during the broader sweep.
- PASS: final helper package tests and `go test -race ./relay/helper -run '^TestHedge' -count=1`.
- PASS: independent relaykit build and `GOWORK=off go test ./dto -run ChannelReliability -count=1`.
- PASS: final nonbillable UI work — 18 tests in three files; typecheck,
  changed-file lint, i18n sync; parent reran `bun run build` successfully.
- PASS: implementation owner ran `go vet` on controller, relay/helper,
  relay/common, service, common, and setting/operation_setting.
- PASS: whitespace check. Final independent review passed with no remaining feature blockers.

## Baseline whole-suite race failures

The broader race sweep passes controller, relay/helper, relay/common, model,
middleware, common, and setting/operation_setting. It fails in two unchanged
legacy test areas:

1. `relay/channel/TestProcessHeaderOverride*`: parallel tests mutate global
   Gin mode through `gin.SetMode`.
2. `service/TestUpdateVideoTasksSlowChannelDoesNotBlockOtherChannels`: its
   assertion reads a Task while GORM mutates the same test object.

Both failures reproduced on an exported clean `HEAD` (`606927f44`), with:

`go test -race ./relay/channel ./service -run 'TestProcessHeaderOverride|TestUpdateVideoTasksSlowChannelDoesNotBlockOtherChannels' -count=1`

No unrelated legacy test changes were made. The clean baseline also exhibited
an old logger race that the task's logger locking changes avoid. This does not
turn a failing full race sweep into a passing one; feature-specific race
validation is recorded separately above.

Detailed local logs:
- `/private/tmp/flowapi-hedge-final-backend.log`
- `/private/tmp/flowapi-hedge-targeted-race.log`
- `/private/tmp/flowapi-hedge-final-race.log`
- `/private/tmp/flowapi-hedge-baseline-race.log`
- `/private/tmp/flowapi-hedge-final-build.log`
