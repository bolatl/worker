# Design Write-up

## 1. What delivery guarantee does your system provide (at-most-once / at-least-once / exactly-once) and why?

**At-least-once.**

RabbitMQ with manual acknowledgements can redeliver a message if the worker dies before ACKing. We never ACK until the job has reached a terminal state in Postgres. So a message may be delivered more than once (e.g. worker crash, network hiccup). We do not provide exactly-once—duplicates are possible—but we ensure no message is lost.

---

## 2. When do you mark a message as successfully processed (from RabbitMQ's perspective), and why is that point safe?

We ACK (mark successfully processed) only when the job is in a **terminal state** in Postgres: `done` or `failed`.

Specifically, we ACK when:
- **Success:** We have committed `MarkDone` to Postgres (job status = done, result stored).
- **Poison job:** We have committed `RecordFailure` with `attempts >= max_attempts` (job status = failed).
- **Duplicate delivery:** `TryMarkProcessing` sees the job is already done or failed—we do no work, just ACK to remove the duplicate message.

This is safe because we never ACK until the database state is committed. If we crashed after the DB commit but before ACK, the message would be redelivered; the next worker would see the terminal state, perform an idempotent no-op, and ACK. No double-processing and no data loss.

---

## 3. How do you bound retries and ensure poison jobs reach a terminal `failed` state (no infinite retry loop)?

**Bounded retries:**
- Each job has `max_attempts` (default 5).
- `RecordFailure` increments `attempts` when a job fails logically. When `attempts >= max_attempts`, the job is marked `failed` and we ACK the message (no more retries).
- The reaper also increments `attempts` when requeuing jobs stuck in processing. After `max_attempts` reaper interventions, the job is marked `failed` instead of requeued.

**Poison job flow:**
1. Worker processes job → job fails → `RecordFailure` → `attempts++`, status set to `queued` (or `failed` if `attempts >= max_attempts`).
2. Consumer NACKs with requeue (500ms delay to avoid a hot loop).
3. Message is redelivered; steps repeat until `attempts >= max_attempts`.
4. On the final failure, status = `failed`, we ACK → message removed, no further retries.

**No infinite loop:** Once `attempts >= max_attempts`, we always ACK. The 500ms sleep before NACK further prevents tight retry loops.

---

## 4. How do you avoid corrupting state if a message is delivered twice (idempotency / duplicate-safety approach)?

**Database as source of truth:**
- Before doing any work, we call `TryMarkProcessing`, which uses `SELECT ... FOR UPDATE` to lock the row.
- If the job is already `done` or `failed`, we return immediately without changing state. The consumer treats this as terminal and ACKs.
- If the job is `processing` (another worker has it, or a dead worker left it stuck), we do not take it; we NACK and let the reaper or the other worker handle it.

**Idempotent updates:**
- `MarkDone` uses `WHERE status IN (queued, processing)`, so it never overwrites a job already done or failed.
- `RecordFailure` checks for terminal status and returns early if the job is already done or failed.

**Result:** Duplicate deliveries are harmless. We either skip work (terminal state) or safely retry (non-terminal). State is never corrupted.

---

## 5. What happens when the worker dies mid-processing? Describe the expected behavior and recovery path.

**Scenario:** Worker receives a message, marks the job `processing`, and is killed before completing (e.g. SIGKILL, OOM).

**Behavior:**
1. The message was never ACKed. RabbitMQ redelivers it when a new consumer connects.
2. The job remains `status = processing` in Postgres with `processing_started_at` set.
3. New worker receives the redelivered message.
4. `TryMarkProcessing` sees `processing` → does not take the job (avoids double-processing a job another worker might still be handling).
5. Consumer NACKs with requeue. The message cycles until the reaper runs.
6. **Reaper** (every 10 seconds) finds jobs in `processing` for longer than `PROCESSING_TIMEOUT` (default 60s). It increments `attempts` and either:
   - Requeues the job (if `attempts < max_attempts`), or
   - Marks it `failed` (if `attempts >= max_attempts`).
7. Once requeued, the next delivery is handled normally: worker takes the job and processes it.
8. If the job keeps getting stuck (worker repeatedly killed), the reaper will eventually mark it `failed` after `max_attempts` interventions.
9. When the job is `failed`, the next delivery results in an idempotent no-op and ACK; the message is removed.

**Recovery path:** Job either completes successfully after reaper requeue, or reaches `failed` after `max_attempts` stuck cycles. No silent loss.
