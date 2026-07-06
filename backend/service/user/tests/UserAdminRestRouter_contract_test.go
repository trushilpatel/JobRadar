package tests

import (
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// TestUserAdminRest_ContractAppearsInOpenAPI is Slice 11's per-slice quality
// gate contract test. Also guards the literal-vs-{id} ordering (/users/{id}/roles,
// /users/{id}/status) — both must appear distinctly from the bare /{id} DELETE.
func TestUserAdminRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := usersvc.NewAdminRestService(&fakeAdminRepo{})
	router := usersvc.NewAdminRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	listItem, ok := zoneCfg.Paths["/users"]
	if !ok || listItem.Get == nil {
		t.Fatalf("expected GET /users in the generated spec, got: %v", zoneCfg.Paths)
	}
	if listItem.Get.OperationID != "listUsersAdmin" {
		t.Errorf("list OperationID = %q, want listUsersAdmin", listItem.Get.OperationID)
	}

	idItem, ok := zoneCfg.Paths["/users/{id}"]
	if !ok || idItem.Get == nil || idItem.Delete == nil {
		t.Fatalf("expected GET+DELETE /users/{id} in the generated spec, got: %v", zoneCfg.Paths)
	}
	if idItem.Get.OperationID != "getUserAdmin" {
		t.Errorf("get OperationID = %q, want getUserAdmin", idItem.Get.OperationID)
	}
	if idItem.Delete.OperationID != "deleteUserAdmin" {
		t.Errorf("delete OperationID = %q, want deleteUserAdmin", idItem.Delete.OperationID)
	}

	rolesItem, ok := zoneCfg.Paths["/users/{id}/roles"]
	if !ok || rolesItem.Patch == nil {
		t.Fatalf("expected PATCH /users/{id}/roles in the generated spec, got: %v", zoneCfg.Paths)
	}
	if rolesItem.Patch.OperationID != "setUserRoles" {
		t.Errorf("setRoles OperationID = %q, want setUserRoles", rolesItem.Patch.OperationID)
	}

	statusItem, ok := zoneCfg.Paths["/users/{id}/status"]
	if !ok || statusItem.Patch == nil {
		t.Fatalf("expected PATCH /users/{id}/status in the generated spec, got: %v", zoneCfg.Paths)
	}
	if statusItem.Patch.OperationID != "setUserStatus" {
		t.Errorf("setStatus OperationID = %q, want setUserStatus", statusItem.Patch.OperationID)
	}
}
