package middleware

import "maps"

// Local fork routes declare their token scopes here instead of appending
// entries to accessTokenRouteRules in access_token_routes.go, which stays
// identical to upstream. The map is package-level and this init runs during
// package initialization, before any request matches a rule. If upstream ever
// changes the "METHOD /path" key format, these entries stop matching silently;
// TestAccessTokenRouteRulesCoverEveryDashboardRoute fails on any route that
// loses its rule, which is the guard for that drift.
func init() {
	maps.Copy(accessTokenRouteRules, map[string]accessTokenRouteRule{
		"GET /api/quota-management/overview":                              accessTokenScopeRule("billing:read"),
		"GET /api/quota-management/cycles":                                accessTokenScopeRule("billing:read"),
		"POST /api/quota-management/cycles":                               accessTokenScopeRule("billing:write"),
		"GET /api/quota-management/cycles/:id":                            accessTokenScopeRule("billing:read"),
		"PATCH /api/quota-management/cycles/:id":                          accessTokenScopeRule("billing:write"),
		"POST /api/quota-management/cycles/:id/close":                     accessTokenScopeRule("billing:write"),
		"GET /api/quota-management/plans/options":                         accessTokenScopeRule("billing:read"),
		"POST /api/quota-management/plans":                                accessTokenScopeRule("billing:write"),
		"GET /api/quota-management/plans/:id":                             accessTokenScopeRule("billing:read"),
		"POST /api/quota-management/plans/:id/execute":                    accessTokenScopeRule("billing:write"),
		"POST /api/quota-management/plans/:id/cancel":                     accessTokenScopeRule("billing:write"),
		"POST /api/quota-management/plans/:id/regenerate":                 accessTokenScopeRule("billing:write"),
		"POST /api/quota-management/plans/:id/notifications/retry":        accessTokenScopeRule("billing:write"),
		"GET /api/quota-management/temporary-quota-requests":              accessTokenScopeRule("billing:read"),
		"POST /api/quota-management/temporary-quota-requests/:id/approve": accessTokenScopeRule("billing:write"),
		"POST /api/quota-management/temporary-quota-requests/:id/reject":  accessTokenScopeRule("billing:write"),
		"GET /api/user/temporary-quota":                                   accessTokenScopeRule("wallet:read"),
		"POST /api/user/temporary-quota":                                  accessTokenScopeRule("wallet:write"),
		"GET /api/user/temporary-quota/requests":                          accessTokenScopeRule("wallet:read"),
		"POST /api/integrations/launch":                                   accessTokenScopeRule("profile:write"),
		"GET /api/user/:id/limits":                                        accessTokenScopeRule("user:read"),
		"PUT /api/user/:id/limits":                                        accessTokenScopeRule("user:write"),
		"DELETE /api/user/:id/access-sources":                             accessTokenScopeRule("user:write"),
		"POST /api/user/:id/access-sources/allow":                         accessTokenScopeRule("user:write"),
	})
}
