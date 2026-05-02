# Cancel generation

> Superpowers plan for adding a user-facing cancel button to active image generation tasks.

**Goal:** Let users cancel a freshly submitted queued generation after submitting the wrong prompt, release the active-task lock, refund the credit, and avoid sending the task upstream.

- [x] Add a backend cancel action for queued generation tasks.
- [x] Keep cancellation idempotent with existing generation refund ledger behavior.
- [x] Add a short worker claim delay so the cancel window is real.
- [x] Reject cancellation once a task is running because upstream consumption may already have started.
- [x] Add a workspace cancel button and canceled-state guidance.
- [x] Cover the flow with backend and frontend tests.
