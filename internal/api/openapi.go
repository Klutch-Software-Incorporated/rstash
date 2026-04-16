package api

import (
	"encoding/json"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

// AdminOpenAPISpec builds the OpenAPI 3.0 spec for the admin API programmatically.
func AdminOpenAPISpec() *openapi3.T {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:       "rstash Admin API",
			Description: "API for managing users and quotas on an rstash remoteStorage server.",
			Version:     "1.0.0",
		},
		Security: openapi3.SecurityRequirements{
			{"ApiKeyAuth": {}},
		},
		Components: &openapi3.Components{
			SecuritySchemes: openapi3.SecuritySchemes{
				"ApiKeyAuth": &openapi3.SecuritySchemeRef{
					Value: &openapi3.SecurityScheme{
						Type: "apiKey",
						In:   "header",
						Name: "X-API-Key",
					},
				},
			},
			Schemas: openapi3.Schemas{
				"User": &openapi3.SchemaRef{Value: userSchema()},
				"Stats": &openapi3.SchemaRef{Value: statsSchema()},
				"Error": &openapi3.SchemaRef{Value: errorSchema()},
			},
		},
	}

	spec.Paths = &openapi3.Paths{}

	spec.Paths.Set("/api/admin/users", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Summary:     "List users",
			OperationID: "listUsers",
			Description: "Returns all users with storage statistics.",
			Responses:   openapi3.NewResponses(withResponse(200, "User list", listUsersResponseSchema())),
		},
		Post: &openapi3.Operation{
			Summary:     "Create user",
			OperationID: "createUser",
			Description: "Creates a new user account.",
			RequestBody: &openapi3.RequestBodyRef{Value: jsonBody(createUserSchema())},
			Responses:   openapi3.NewResponses(withResponse(201, "Created user", singleUserResponseSchema())),
		},
	})

	spec.Paths.Set("/api/admin/users/{username}", &openapi3.PathItem{
		Parameters: []*openapi3.ParameterRef{usernameParam()},
		Get: &openapi3.Operation{
			Summary:     "Get user",
			OperationID: "getUser",
			Description: "Returns a single user with storage statistics.",
			Responses:   openapi3.NewResponses(withResponse(200, "User details", singleUserResponseSchema())),
		},
		Delete: &openapi3.Operation{
			Summary:     "Delete user",
			OperationID: "deleteUser",
			Description: "Permanently deletes a user account.",
			Responses:   openapi3.NewResponses(withResponse(200, "User deleted", okResponseSchema())),
		},
	})

	spec.Paths.Set("/api/admin/users/{username}/quota", &openapi3.PathItem{
		Parameters: []*openapi3.ParameterRef{usernameParam()},
		Put: &openapi3.Operation{
			Summary:     "Set user quota",
			OperationID: "setUserQuota",
			Description: "Sets the storage quota for a user in bytes. Set to 0 for server default.",
			RequestBody: &openapi3.RequestBodyRef{Value: jsonBody(quotaRequestSchema())},
			Responses:   openapi3.NewResponses(withResponse(200, "Quota updated", okResponseSchema())),
		},
	})

	spec.Paths.Set("/api/admin/users/{username}/disable", &openapi3.PathItem{
		Parameters: []*openapi3.ParameterRef{usernameParam()},
		Post: &openapi3.Operation{
			Summary:     "Disable user",
			OperationID: "disableUser",
			Description: "Marks a user account disabled. Idempotent. No request body.",
			Responses:   openapi3.NewResponses(withResponse(200, "User disabled", okResponseSchema())),
		},
	})

	spec.Paths.Set("/api/admin/users/{username}/enable", &openapi3.PathItem{
		Parameters: []*openapi3.ParameterRef{usernameParam()},
		Post: &openapi3.Operation{
			Summary:     "Enable user",
			OperationID: "enableUser",
			Description: "Clears the disabled flag on a user account. Idempotent. No request body.",
			Responses:   openapi3.NewResponses(withResponse(200, "User enabled", okResponseSchema())),
		},
	})

	spec.Paths.Set("/api/admin/stats", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Summary:     "Server statistics",
			OperationID: "getStats",
			Description: "Returns server-wide statistics: total users, storage, and documents.",
			Responses:   openapi3.NewResponses(withResponse(200, "Server stats", statsResponseSchema())),
		},
	})

	return spec
}

// ServeOpenAPISpec returns a handler that serves the admin API OpenAPI spec as JSON.
func ServeOpenAPISpec(spec *openapi3.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spec)
	}
}

// --- Schema helpers ---

func userSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: openapi3.Schemas{
			"username":      schemaRef("string", ""),
			"email":         schemaRef("string", ""),
			"is_admin":      schemaRef("boolean", ""),
			"disabled":      schemaRef("boolean", ""),
			"approved":      schemaRef("boolean", ""),
			"storage_quota": schemaRef("integer", "int64"),
			"storage_used":  schemaRef("integer", "int64"),
			"file_count":    schemaRef("integer", "int64"),
			"created_at":    schemaRef("string", "date-time"),
			"last_login_at": schemaRef("string", "date-time"),
		},
	}
}

func statsSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: openapi3.Schemas{
			"total_users":         schemaRef("integer", "int64"),
			"total_storage_bytes": schemaRef("integer", "int64"),
			"total_documents":     schemaRef("integer", "int64"),
		},
	}
}

func errorSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: openapi3.Schemas{
			"error": schemaRef("string", ""),
		},
	}
}

func createUserSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"username"},
		Properties: openapi3.Schemas{
			"username":              schemaRef("string", ""),
			"password":              schemaRef("string", ""),
			"email":                 schemaRef("string", ""),
			"email_verified":        schemaRef("boolean", ""),
			"provision":             schemaRef("boolean", ""),
			"quota_bytes":           quotaFieldRef("Storage quota in bytes. 0 = use server default."),
			"bandwidth_quota_bytes": quotaFieldRef("Monthly egress quota in bytes. 0 = use server default."),
		},
	}
}

func quotaRequestSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"quota_bytes"},
		Properties: openapi3.Schemas{
			"quota_bytes": quotaFieldRef("Storage quota in bytes. 0 = use server default."),
		},
	}
}

func listUsersResponseSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: openapi3.Schemas{
			"data": {Value: &openapi3.Schema{
				Type:  &openapi3.Types{"array"},
				Items: &openapi3.SchemaRef{Ref: "#/components/schemas/User"},
			}},
			"total": schemaRef("integer", ""),
		},
	}
}

func singleUserResponseSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: openapi3.Schemas{
			"data": {Ref: "#/components/schemas/User"},
		},
	}
}

func statsResponseSchema() *openapi3.Schema {
	return statsSchema()
}

func okResponseSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: openapi3.Schemas{
			"ok": schemaRef("string", ""),
		},
	}
}

func schemaRef(typ, format string) *openapi3.SchemaRef {
	s := &openapi3.Schema{Type: &openapi3.Types{typ}}
	if format != "" {
		s.Format = format
	}
	return &openapi3.SchemaRef{Value: s}
}

// quotaFieldRef returns the schema for a quota field (storage or bandwidth).
// Values must be non-negative; zero has the special meaning "use the server
// default" (quota_user or bandwidth_quota_user settings).
func quotaFieldRef(description string) *openapi3.SchemaRef {
	zero := float64(0)
	return &openapi3.SchemaRef{Value: &openapi3.Schema{
		Type:        &openapi3.Types{"integer"},
		Format:      "int64",
		Min:         &zero,
		Description: description,
	}}
}

func usernameParam() *openapi3.ParameterRef {
	return &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:     "username",
			In:       "path",
			Required: true,
			Schema:   schemaRef("string", ""),
		},
	}
}

func jsonBody(schema *openapi3.Schema) *openapi3.RequestBody {
	return &openapi3.RequestBody{
		Required: true,
		Content: openapi3.Content{
			"application/json": &openapi3.MediaType{
				Schema: &openapi3.SchemaRef{Value: schema},
			},
		},
	}
}

func withResponse(status int, desc string, schema *openapi3.Schema) openapi3.NewResponsesOption {
	return openapi3.WithStatus(status, &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: &desc,
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{Value: schema},
				},
			},
		},
	})
}
