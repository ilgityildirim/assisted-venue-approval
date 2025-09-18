package trust

import (
	"testing"

	"assisted-venue-approval/internal/models"
)

func TestAssess_Admin(t *testing.T) {
	c := NewDefault()
	u := models.User{IsVenueAdmin: true}
	a := c.Assess(u, "")
	if a.Trust != 1.0 || a.Authority != "venue_admin" || a.Bonus != c.cfg.BonusVenueAdmin {
		t.Fatalf("unexpected assessment: %+v", a)
	}
}

func TestAssess_Ambassador_High_WithRegion(t *testing.T) {
	c := NewDefault()
	lvl := 3
	pts := 1500
	reg := "Seoul"
	u := models.User{AmbassadorLevel: &lvl, AmbassadorPoints: &pts, AmbassadorRegion: &reg}
	a := c.Assess(u, "Gangnam-gu, Seoul, South Korea")
	if a.Authority != "high_ambassador" {
		t.Fatalf("expected high_ambassador, got %+v", a)
	}
	if a.Trust < 0.79 { // allow float rounding
		t.Fatalf("expected trust around 0.8, got %v", a.Trust)
	}
	if a.Bonus != c.cfg.BonusHighAmb {
		t.Fatalf("unexpected bonus: %+v", a)
	}
}

func TestAssess_Ambassador_Regular_NoRegion(t *testing.T) {
	c := NewDefault()
	lvl := 2
	pts := 400
	reg := "Tokyo"
	u := models.User{AmbassadorLevel: &lvl, AmbassadorPoints: &pts, AmbassadorRegion: &reg}
	a := c.Assess(u, "Seoul, South Korea")
	if a.Authority != "ambassador" {
		t.Fatalf("expected ambassador, got %+v", a)
	}
	if a.Trust < 0.59 || a.Trust >= 0.7 {
		t.Fatalf("expected trust around 0.6 with boosts, got %v", a.Trust)
	}
	if a.Bonus != c.cfg.BonusAmb {
		t.Fatalf("unexpected bonus: %+v", a)
	}
}

func TestAssess_Trusted_WithContribBoosts(t *testing.T) {
	c := NewDefault()
	u := models.User{Trusted: true, Contributions: 600}
	a := c.Assess(u, "")
	// 0.7 + 0.1 + 0.1 = 0.9
	if a.Authority != "trusted" {
		t.Fatalf("expected trusted, got %+v", a)
	}
	if a.Trust < 0.89 || a.Trust > 0.91 {
		t.Fatalf("expected ~0.9 trust, got %v", a.Trust)
	}
	if a.Bonus != c.cfg.BonusTrusted {
		t.Fatalf("unexpected bonus: %+v", a)
	}
}

func TestAssess_Regular_CapAtOne(t *testing.T) {
	c := NewDefault()
	u := models.User{Contributions: 10000}
	a := c.Assess(u, "")
	if a.Authority != "regular" {
		t.Fatalf("expected regular, got %+v", a)
	}
	if a.Trust != 0.5 { // 0.3 + 0.1 + 0.1
		t.Fatalf("expected 0.5 trust, got %v", a.Trust)
	}
	if a.Bonus != c.cfg.BonusRegular {
		t.Fatalf("unexpected bonus: %+v", a)
	}
}
