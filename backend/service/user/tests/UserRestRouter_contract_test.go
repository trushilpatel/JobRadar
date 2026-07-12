package tests

import (
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// TestUserRest_ContractAppearsInOpenAPI is the Slice 7 per-slice quality
// gate's contract test. It also guards the middleware-self-injection fix:
// registering directly on the passed router (no nested "/me" mount) must
// produce absolute-within-zone paths ("/me", "/me/config"), not relative
// ones ("/", "/config") that would merge into the combined spec wrong.
func TestUserRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := usersvc.NewRestService(&stubService{})
	router := usersvc.NewRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	meItem, ok := zoneCfg.Paths["/me"]
	if !ok || meItem.Get == nil || meItem.Patch == nil || meItem.Delete == nil {
		t.Fatalf("expected GET+PATCH+DELETE /me in the generated spec, got: %v", zoneCfg.Paths)
	}
	if meItem.Get.OperationID != "getMe" {
		t.Errorf("get OperationID = %q, want getMe", meItem.Get.OperationID)
	}
	if meItem.Patch.OperationID != "updateMe" {
		t.Errorf("patch OperationID = %q, want updateMe", meItem.Patch.OperationID)
	}
	if meItem.Delete.OperationID != "deleteMe" {
		t.Errorf("delete OperationID = %q, want deleteMe", meItem.Delete.OperationID)
	}

	configItem, ok := zoneCfg.Paths["/me/config"]
	if !ok || configItem.Get == nil || configItem.Patch == nil {
		t.Fatalf("expected GET+PATCH /me/config in the generated spec, got: %v", zoneCfg.Paths)
	}
	if configItem.Get.OperationID != "getMyConfig" {
		t.Errorf("get config OperationID = %q, want getMyConfig", configItem.Get.OperationID)
	}
	if configItem.Patch.OperationID != "updateMyConfig" {
		t.Errorf("patch config OperationID = %q, want updateMyConfig", configItem.Patch.OperationID)
	}

	// Guards against the exact bug the old nested-mount design would have
	// produced when merged into the combined spec.
	if _, ok := zoneCfg.Paths["/"]; ok {
		t.Error("expected no relative \"/\" path — /me must be registered absolute-within-zone")
	}
}
