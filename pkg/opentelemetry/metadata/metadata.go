package wotelmeta

import "time"

const (
	Scheme                     = "scheme"
	PerformedAt                = "performed_at"
	MessageKind                = "message_kind"
	CorrelationID              = "correlation_id"
	ReplyTo                    = "reply_to"
	ReplyPartition             = "reply_partition"
	ExpiresAt                  = "expires_at"
	MessageKindEvent           = "event"
	MessageKindCommand         = "command"
	MessageKindCommandResponse = "command_response"
)

func MakePerformedAt(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

func ParsePerformedAt(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}
