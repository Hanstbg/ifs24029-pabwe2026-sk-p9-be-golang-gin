// Package docs menyimpan spesifikasi OpenAPI/Swagger Delcom Blog API.
package docs

import (
	_ "embed"
	"encoding/json"
)

//go:embed swagger.json
var swaggerJSON []byte

// GetSwaggerJSON mengembalikan spesifikasi OpenAPI dengan host dan scheme
// yang disesuaikan secara dinamis dengan pemanggil (agar "Try it out" tepat sasaran).
func GetSwaggerJSON(host, scheme string) string {
	var spec map[string]interface{}
	if err := json.Unmarshal(swaggerJSON, &spec); err != nil {
		return string(swaggerJSON)
	}
	spec["host"] = host
	spec["schemes"] = []string{scheme}
	out, err := json.Marshal(spec)
	if err != nil {
		return string(swaggerJSON)
	}
	return string(out)
}
