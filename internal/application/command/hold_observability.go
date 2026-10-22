package command

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
)

func observeHoldCommand(
	ctx context.Context,
	started time.Time,
	span application.Span,
	logger application.Logger,
	metrics application.Metrics,
	commandName, state string,
	replay bool,
	commandErr error,
) {
	defer span.End()
	metrics.RecordCommandDuration(commandName, float64(time.Since(started).Milliseconds()))
	if commandErr != nil {
		span.RecordError(commandErr)
		if errors.Is(commandErr, hold.ErrHoldConflict) || errors.Is(commandErr, hold.ErrReleaseConflict) {
			state = "conflict"
			metrics.RecordIdempotencyTotal("conflict")
		} else {
			state = "failure"
		}
		metrics.RecordCommandTotal(commandName, state)
		logger.WithTrace(ctx).Error(commandName+" failed",
			slog.String("trace_id", span.SpanContext().TraceID()),
			slog.String("state", state),
		)
		return
	}

	metrics.RecordCommandTotal(commandName, state)
	if replay {
		metrics.RecordIdempotencyTotal("replay")
	} else {
		metrics.RecordIdempotencyTotal("new")
	}
	logger.WithTrace(ctx).Info(commandName+" completed",
		slog.String("trace_id", span.SpanContext().TraceID()),
		slog.String("state", state),
		slog.Bool("idempotent_replay", replay),
	)
}
