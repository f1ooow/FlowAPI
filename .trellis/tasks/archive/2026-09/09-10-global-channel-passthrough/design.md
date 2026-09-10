# Design

## Data Flow

Keep global.pass_through_request_enabled and its existing OR relationship with the local body flag. Add global.pass_through_headers_enabled to registered GlobalSettings with a false zero value, using the existing option persistence system.

GetEffectiveHeaderOverride adds a wildcard to a fresh sanitized map when enabled. Runtime maps are final and must not be repopulated. Channel tests must not forward administrator request headers. No channel database migration or backfill is needed.

GET /api/channel/passthrough uses channel-read permission and returns only the two flags. React Query shares it between table and form; option saves invalidate it.

## Header Contract

Retain adapter setup followed by passthrough and explicit overrides. Exclude Content-Type and chatgpt-account-id from automatic wildcard/regex forwarding alongside existing credentials and transport protections. Explicit administrator entries can still override them. Test actual outgoing HTTP requests with real adapter setup and a local upstream.

## Interface

Independently save precisely named body/header switches. Show inherited state in channel forms and rows. Disable the displayed-on body switch while inherited without changing its underlying local form value. Use existing components and localization conventions.

## Limits And Rollback

Header protection additions also affect existing wildcard/regex templates. Body passthrough remains original bytes and may bypass model mapping, parameter overrides, system prompts or protocol conversion. Header inheritance covers existing shared forwarding machinery; inspect and report bespoke transport exceptions. Do not claim universal raw HTTP proxying.

No channel-local opt-out is added. Common credential exclusions cannot recognize every custom secret; ordinary metadata reaches upstream and cache hits remain provider-dependent. Disable the global header option to restore local-only behavior. Do not deploy or alter production settings.
