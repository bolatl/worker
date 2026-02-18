package queue

// JobMessage is the JSON payload sent to RabbitMQ containing the job ID.
type JobMessage struct {
	JobID string `json:"job_id"`
}
