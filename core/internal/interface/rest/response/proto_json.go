// Package response provides helpers for serializing API responses.
package response

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/labstack/echo/v4"
)

var marshaler = protojson.MarshalOptions{
	EmitUnpopulated: false,
	UseProtoNames:   false, // keeps camelCase field names
}

// ProtoJSON marshals a protobuf message using protojson and writes it as JSON.
// Timestamps are serialized as RFC3339 strings, not as {"seconds":...,"nanos":...} objects.
// Use this for all protobuf response bodies.
func ProtoJSON(c echo.Context, status int, msg proto.Message) error {
	b, err := marshaler.Marshal(msg)
	if err != nil {
		return c.JSON(500, map[string]any{"error": map[string]any{
			"code":    "INTERNAL_ERROR",
			"message": "response serialization failed",
		}})
	}
	return c.JSONBlob(status, b)
}
