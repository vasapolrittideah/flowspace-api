-- +goose Up
-- Both columns are nullable, so a running older relay keeps working, and
-- events from before this migration have no stored trace context.
ALTER TABLE identity_outbox_events
    ADD COLUMN traceparent text,
    ADD COLUMN tracestate text;

-- +goose Down
-- This loses only the stored trace context of the events.
ALTER TABLE identity_outbox_events
    DROP COLUMN tracestate,
    DROP COLUMN traceparent;
