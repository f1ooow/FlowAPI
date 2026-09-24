# Media verification

Focused provider tests passed with the workspace Go cache redirected into `/tmp` (the default cache under `~/Library` is not writable):

```text
gocache=/tmp/flowapi-media-go-cache go test ./relay/channel/task/viapi ./relay/channel/task/volcvod ./relay/channel/task/volccv ./relay/channel/minimax ./relay/channel/volcengine ./controller ./model -run 'TestMiniMax|TestVolcengine|TestVOD|TestViapi|TestPreprocess'
PASS: viapi, volcvod, volccv, minimax, volcengine, controller
```

The provider packages also compile and their package tests pass without a test-name filter. Full `./controller` and `./model` suites were attempted; unrelated existing tests that open localhost listeners/miniredis fail in this sandbox (`operation not permitted`), so this is not reported as a full-suite pass.

No live provider calls, paid requests, deployment, or commit were performed.
