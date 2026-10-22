package http

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

func TestGroupPermissionsUseJWTIdentityAndGroupRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("group-jwt-test-secret")
	tokenFor := func(userID, role string) string {
		t.Helper()
		claims := middleware.ClaimsPersonalizadas{
			UserID: userID,
			Role:   role,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "identity",
				Audience:  jwt.ClaimStrings{"student-platform"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	store := service.NewMemoryGroupStore()
	router := gin.New()
	protected := router.Group("")
	protected.Use(middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret: secret, Issuer: "identity", Audience: "student-platform",
	}).RequireAuth())
	NewGroupHandler(service.NewGroupService(store, nil)).RegisterRoutes(protected)
	e := &env{router: router, store: store}
	ownerToken := tokenFor(hAdmin, "member")

	w := e.do(http.MethodPost, "/groups", ownerToken, map[string]any{
		"name": "Grupo JWT", "user_id": hStrang, "owner_user_id": hStrang,
	})
	expect(t, w, http.StatusCreated)
	groupID := decode(t, w)["group_id"].(string)
	role, err := store.GetMemberRole(context.Background(), groupID, hAdmin)
	if err != nil || role != model.RoleAdmin {
		t.Fatalf("el creador del JWT debe ser admin del grupo: role=%q err=%v", role, err)
	}
	if member, err := store.IsMember(context.Background(), groupID, hStrang); err != nil || member {
		t.Fatalf("user_id del body no debe incorporarse: miembro=%v err=%v", member, err)
	}

	// El rol del token no concede administración de grupos ajenos.
	outsiderToken := tokenFor(hStrang, "admin")
	expect(t, e.do(http.MethodGet, "/groups/"+groupID, outsiderToken, nil), http.StatusForbidden)
	expect(t, e.do(http.MethodPost, "/groups/"+groupID+"/invite/regenerate", outsiderToken, nil), http.StatusForbidden)
	expect(t, e.do(http.MethodPost, "/groups/"+groupID+"/invite/regenerate", ownerToken, nil), http.StatusOK)
}
