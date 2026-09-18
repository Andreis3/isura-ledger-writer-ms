package application

type Metrics interface {
	RecordRequestTotal(router, protocol string, statusCode int)
	RecordDBQueryDuration(database, table, method string, duration float64)
	RecordRequestDuration(router, protocol string, statusCode int, duration float64)
	RecordTransactionTotal(status string)
	RecordCommandTotal(command string, state string)
	RecordCommandDuration(command string, duration float64)
	RecordIdempotencyTotal(result string)
	RecordConcurrencyRetry()
	RecordOutboxTotal(status, eventType string)
}
