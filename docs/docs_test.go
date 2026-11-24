package docs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSwaggerJSONInjectsHostAndScheme(t *testing.T) {
	var spec map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(GetSwaggerJSON("api.contoh.id", "https")), &spec))

	assert.Equal(t, "api.contoh.id", spec["host"])
	assert.Equal(t, []interface{}{"https"}, spec["schemes"])
	assert.Equal(t, "2.0", spec["swagger"])
	assert.NotEmpty(t, spec["paths"])
}

func TestGetSwaggerJSONFallsBackToRawSpecWhenInvalid(t *testing.T) {
	original := swaggerJSON
	swaggerJSON = []byte("{rusak")
	t.Cleanup(func() { swaggerJSON = original })

	assert.Equal(t, "{rusak", GetSwaggerJSON("h", "http"))
}
