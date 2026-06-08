# rive-adk-go

A small Go replica of the Google ADK Go runtime flow described by
`01-runtime-flow-deep-dive.md`.

The target is not API compatibility with `google/adk-go`. It is an educational
runtime skeleton that preserves the Chapter 01 architecture line:

```text
Runner -> Agent -> LLM Flow -> Model/Tool -> Event -> Session
```

The implementation is produced through a Rive workflow:

- OpenCode workers implement the runtime in staged nodes.
- A final Codex steward reviews, fixes, verifies, and commits the result.

