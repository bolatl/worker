## Mini Assignment: API + Worker (RabbitMQ + Postgres + Go)

### Goal

Build a tiny event-driven system with:

* an **API** that creates jobs,
* a **worker** that processes jobs asynchronously via **RabbitMQ**,
* state stored in **Postgres**.

Provide `docker-compose.yml` (or equivalent) so we can run everything with one command, plus sample `curl` commands in `README.md`.

You may use any Go libraries (Gin/GORM optional).

---

## Expectations / Hints (non-prescriptive)

We work with RabbitMQ heavily. We don’t expect a perfect production system, but we *do* want to see correct concepts and reasoning.

### RabbitMQ delivery & crash safety

* Your worker must use **manual acknowledgements** (not auto-ack).
* Use **ack/reject/nack semantics** intentionally to achieve **crash safety**: if the worker is killed mid-processing, the job should not silently disappear and should eventually reach a terminal state (`done` or `failed`).

### Retries & poison jobs

* Implement **bounded retries** so jobs do not fail forever.
* A “poison job” (one that always fails) must eventually reach a **terminal `failed` state** and stop consuming resources in a tight loop.
* You may implement backoff and/or dead-lettering, but the key requirement is **bounded retry + no infinite hot loop**.

### Duplicate delivery & idempotency

* Design for **at-least-once delivery**: RabbitMQ may deliver the same message more than once.
* Your system should remain consistent if a job message is processed multiple times.

### Load distribution

* Configure consumption so that with 2 workers and multiple jobs, both workers do useful work, and a slow worker does not accumulate unbounded in-flight work.

---

# Must-have (expected)

## 1) API service

* `POST /jobs` accepts `{ "type": "...", "payload": {...} }`
* Creates a job record in Postgres with status `queued`
* Publishes a message to RabbitMQ containing at least `job_id`
* Returns `202 Accepted` with `{ "job_id": ... }`

Optional but helpful:

* `GET /jobs/:id` returns job status/result

## 2) Worker service

* Consumes from RabbitMQ
* For each message:

  * Loads the job from Postgres
  * Marks it `processing`
  * Performs some small “work” (sleep, hash, validate JSON, etc.)
  * Stores result and marks job `done`
* Message handling should be reliable under crashes/failures: a submitted job should not disappear without reaching `done` or `failed`.

## 3) Basic failure handling

* Record failure info (e.g., error message, attempt count, timestamps).
* Crash safety: If the worker process is killed while a job is being processed, the system should recover after restart and the job should end up in a correct final state (`done` or `failed`) without silently losing the job, and should not remain stuck forever in `processing`.
* Poison job handling: A job that always fails should eventually reach a terminal state (`failed`) and stop retrying.

---

# Strongly preferred (what we look for)

## 4) Idempotency / duplicate safety

* The system behaves correctly if the same job message is delivered more than once.
* Reprocessing must not corrupt state or produce inconsistent results.

## 5) Reasonable load distribution

* With 2 workers running and multiple jobs submitted, both workers should do useful work (one worker shouldn’t take all jobs while the other is idle, assuming similar processing speed).
* A slow worker should not be flooded with an unbounded amount of in-flight work.

---

# Bonus (not required)

Pick any of these:

1. **DLQ / failed queue**
   Route permanently failing jobs to a dead-letter/failed queue.

2. **Backoff**
   Add increasing delay between retries.

3. **Graceful shutdown**
   Worker stops consuming on SIGINT/SIGTERM, finishes in-flight message safely.

4. **Health endpoints**
   `/healthz` checks DB / RabbitMQ connectivity (simple is fine).

5. **Tests**
   Even one meaningful test (unit or integration) is a plus.

---

# Short write-up (required): `DESIGN.md`

Briefly answer:

* What delivery guarantee does your system provide (at-most-once / at-least-once / exactly-once) and why?
* When do you mark a message as **successfully processed** (from RabbitMQ’s perspective), and why is that point safe?
* How do you bound retries and ensure poison jobs reach a terminal `failed` state (no infinite retry loop)?
* How do you avoid corrupting state if a message is delivered twice (idempotency / duplicate-safety approach)?
* What happens when the worker dies mid-processing? Describe the expected behavior and recovery path.

---

## What we will test

* Happy path: create job → becomes `done`.
* Worker crash: kill worker mid-job → restart → job still completes or fails cleanly (no silent loss).
* Poison job: submit a job type/payload that fails → system reaches `failed` eventually (no infinite loop).
* Two workers: run 2 workers and submit multiple jobs → both workers do work (not all by one).

---

# Evaluation focus

We care more about:

* correct message acknowledgement behavior and reasoning,
* state transitions and handling of failure modes,
* ability to explain tradeoffs clearly,

than perfect architecture.
