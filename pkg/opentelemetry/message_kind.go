package opentelemetry

import (
	"github.com/ThreeDotsLabs/watermill/message"

	wotelmeta "github.com/Gungniir/watermill-opentelemetry/pkg/opentelemetry/metadata"
)

const (
	MessageKindCommand         = wotelmeta.MessageKindCommand
	MessageKindCommandResponse = wotelmeta.MessageKindCommandResponse
	MessageKindEvent           = wotelmeta.MessageKindEvent
)

func SetMessageKind(msg *message.Message, kind string) {
	msg.Metadata.Set(wotelmeta.MessageKind, kind)
}

func messageKind(msg *message.Message, config *config) string {
	kind := msg.Metadata.Get(wotelmeta.MessageKind)
	if kind != "" {
		return kind
	}

	return config.defaultMessageKind
}
