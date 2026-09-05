# Jobs

Jobs represent bounded, potentially long-running generation work that does not fit a persistent chat or audio session. The persisted lifecycle is `queued`, `preparing`, `loading`, `running`, then `completed`, `failed`, or `cancelled`.

Each job records its model, capability, runtime adapter, compute target, timestamps, honest step-based progress, artifacts, and sanitized error. Cancellation propagates through the runner context. On service restart, unfinished jobs are reconciled as failed rather than being reported as still active.

The management API supports create, list, inspect, and cancel under `/api/backpack/v1/jobs`. The public Go client and `backpack jobs` use that contract. Image and video request validation and CLI surfaces exist, but no Z-Image or Wan runner is registered until its trusted package and real GPU path pass execution validation.

Runners implement the existing model, runtime-adapter, and compute-target boundaries. They report steps they actually receive from an engine; the service does not invent percentage precision.
