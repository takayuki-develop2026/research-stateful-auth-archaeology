package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type projectSettingsRow struct {
	AllowedModes  []int
	AllowedRoutes []string
	IsEnabled     bool

	AllowedModesPolicy  string
	AllowedRoutesPolicy string
}

func GateByProjectSettings(
	ctx context.Context,
	db *pgxpool.Pool,
	projectID string,
	mode int,
	routeID string,
) (allowed bool, reason string, detail map[string]any, err error) {

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	projectID = strings.TrimSpace(projectID)
	routeID = strings.TrimSpace(routeID)

	if projectID == "" {
		return false, "settings_missing", map[string]any{
			"project_id": projectID,
			"reason":     "project_id_empty",
		}, nil
	}

	ps, ok, loadErr := loadProjectSettings(ctx, db, projectID)
	if loadErr != nil {
		// DB/JSON破損などは “lookup_failed” として必ず detail に入れる
		return false, "settings_lookup_failed", map[string]any{
			"project_id":   projectID,
			"error_detail": loadErr.Error(),
		}, loadErr
	}
	if !ok {
		return false, "settings_missing", map[string]any{
			"project_id": projectID,
		}, nil
	}
	if !ps.IsEnabled {
		return false, "project_disabled", map[string]any{
			"project_id": projectID,
		}, nil
	}

	// ---- modes gate ----
	switch NormalizePolicy(ps.AllowedModesPolicy) {
	case PolicyAllowAll:
		// pass
	case PolicyDenyAll:
		return false, "mode_not_allowed", map[string]any{
			"project_id":    projectID,
			"mode":          mode,
			"allowed_modes": ps.AllowedModes,
			"policy":        PolicyDenyAll,
		}, nil
	case PolicyAllowList:
		if len(ps.AllowedModes) == 0 {
			return false, "mode_not_allowed", map[string]any{
				"project_id":    projectID,
				"mode":          mode,
				"allowed_modes": ps.AllowedModes,
				"policy":        "allow_list_empty_denies",
			}, nil
		}
		if !ContainsInt(ps.AllowedModes, mode) {
			return false, "mode_not_allowed", map[string]any{
				"project_id":    projectID,
				"mode":          mode,
				"allowed_modes": ps.AllowedModes,
				"policy":        PolicyAllowList,
			}, nil
		}
	default:
		// 不正 policy でも安全に deny
		return false, "mode_not_allowed", map[string]any{
			"project_id": projectID,
			"mode":       mode,
			"policy":     ps.AllowedModesPolicy,
			"reason":     "unknown_policy",
		}, nil
	}

	// ---- routes gate ----
	switch NormalizePolicy(ps.AllowedRoutesPolicy) {
	case PolicyAllowAll:
		// pass
	case PolicyDenyAll:
		return false, "route_not_allowed", map[string]any{
			"project_id":     projectID,
			"route_id":       routeID,
			"allowed_routes": ps.AllowedRoutes,
			"policy":         PolicyDenyAll,
		}, nil
	case PolicyAllowList:
		if routeID == "" {
			return false, "route_not_allowed", map[string]any{
				"project_id":     projectID,
				"route_id":       routeID,
				"allowed_routes": ps.AllowedRoutes,
				"policy":         PolicyAllowList,
				"reason":         "route_id_missing",
			}, nil
		}
		if len(ps.AllowedRoutes) == 0 {
			return false, "route_not_allowed", map[string]any{
				"project_id":     projectID,
				"route_id":       routeID,
				"allowed_routes": ps.AllowedRoutes,
				"policy":         "allow_list_empty_denies",
			}, nil
		}
		if !ContainsStr(ps.AllowedRoutes, routeID) {
			return false, "route_not_allowed", map[string]any{
				"project_id":     projectID,
				"route_id":       routeID,
				"allowed_routes": ps.AllowedRoutes,
				"policy":         PolicyAllowList,
			}, nil
		}
	default:
		return false, "route_not_allowed", map[string]any{
			"project_id": projectID,
			"route_id":   routeID,
			"policy":     ps.AllowedRoutesPolicy,
			"reason":     "unknown_policy",
		}, nil
	}

	return true, "", nil, nil
}

func loadProjectSettings(ctx context.Context, db *pgxpool.Pool, projectID string) (projectSettingsRow, bool, error) {
	var allowedModesJSON string
	var allowedRoutesJSON string
	var isEnabled bool
	var modesPolicy string
	var routesPolicy string

	err := db.QueryRow(ctx, `
		SELECT
			allowed_modes::text   AS allowed_modes_json,
			allowed_routes::text  AS allowed_routes_json,
			is_enabled            AS is_enabled,
			allowed_modes_policy  AS allowed_modes_policy,
			allowed_routes_policy AS allowed_routes_policy
		FROM project_settings
		WHERE project_id = $1
	`, projectID).Scan(&allowedModesJSON, &allowedRoutesJSON, &isEnabled, &modesPolicy, &routesPolicy)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return projectSettingsRow{}, false, nil
		}
		return projectSettingsRow{}, false, err
	}

	// jsonb -> text なので "[]" や "[1,2]" の形で来る前提
	var modes []int
	if umErr := json.Unmarshal([]byte(allowedModesJSON), &modes); umErr != nil {
		return projectSettingsRow{}, true, fmt.Errorf("invalid allowed_modes json: %w (raw=%s)", umErr, allowedModesJSON)
	}

	var routes []string
	if urErr := json.Unmarshal([]byte(allowedRoutesJSON), &routes); urErr != nil {
		return projectSettingsRow{}, true, fmt.Errorf("invalid allowed_routes json: %w (raw=%s)", urErr, allowedRoutesJSON)
	}

	modesPolicy = strings.TrimSpace(modesPolicy)
	routesPolicy = strings.TrimSpace(routesPolicy)
	if modesPolicy == "" {
		modesPolicy = PolicyAllowList
	}
	if routesPolicy == "" {
		routesPolicy = PolicyAllowList
	}

	return projectSettingsRow{
		AllowedModes:        modes,
		AllowedRoutes:       routes,
		IsEnabled:           isEnabled,
		AllowedModesPolicy:  modesPolicy,
		AllowedRoutesPolicy: routesPolicy,
	}, true, nil
}