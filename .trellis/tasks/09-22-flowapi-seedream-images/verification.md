# Images verification (2026-09-22)

Implementation in owned files is ready for parent integration. No provider calls, frontend changes, commits, or production mutation.

## Passed

Using `GOCACHE=/private/tmp/flowapi-images-go-cache`:

- `go test ./relay/channel/openai -run 'TestSeedream|Test.*Image' -count=1`
- `go test ./relay/helper -run 'TestSeedream|Test.*Image' -count=1` (part of combined run)
- `go test ./service -run 'TestSeedream' -count=1`
- `go test ./service -run 'TestSeedream|Test.*Image' -count=1` (part of combined run)
- `go test relay/image_seedream.go relay/image_seedream_test.go -count=1`
- `go test relay/channel/volcengine/seedream.go relay/channel/volcengine/seedream_test.go -count=1`
- `go vet ./relay/channel/openai ./relay/helper ./service`
- `go vet relay/image_seedream.go relay/image_seedream_test.go`
- `go vet relay/channel/volcengine/seedream.go relay/channel/volcengine/seedream_test.go`
- `cd relaykit && GOWORK=off go build ./...`
- gofmt on all owned Go files, and `git diff --check`.

File-list test commands compile and test the actual production image preparation/native conversion functions, without unrelated unfinished task adapters. They do not certify root integration.

## Integration currently blocked by other work in progress

Combined `go test ./relay/channel/openai ./relay/channel/volcengine ./relay/helper ./relay ./service -run 'TestSeedream|Test.*Image' -count=1` last passed OpenAI/helper/service but could not compile root relay/volcengine due to root DTO references in the ongoing viapi/volcvod adapters (`dto.NewOpenAIVideo`, `dto.VideoStatus*`). Earlier task adapter helper errors are tracked by main too. No other worker files were modified to make these tests pass.

Parent should rerun root/volcengine image tests after adapter imports finish. `seedream_endpoint_test.go` is waiting on that package compilation. Main also owns model registration and the new native conversion call; exact changes are in integration-requests.md.

## Files owned/changed by images

- relaykit/dto/openai_image.go
- relaykit/dto/seedream_image.go (new)
- relay/channel/openai/relay_image.go
- relay/channel/openai/seedream_image_test.go (new)
- relay/channel/volcengine/seedream.go (new)
- relay/channel/volcengine/seedream_test.go (new)
- relay/channel/volcengine/seedream_endpoint_test.go (new)
- relay/image_handler.go
- relay/image_seedream.go (new)
- relay/image_seedream_test.go (new)
- service/seedream_image_billing_test.go (new)
- This task's integration-requests.md, billing-contract.md and verification.md.
