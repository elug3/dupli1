package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// BannerPlacement is where on the storefront a banner shows.
type BannerPlacement string

const (
	// BannerPlacementHomeHero is the full-width image at the top of the home
	// page. With no banner shown there the storefront keeps its built-in hero.
	BannerPlacementHomeHero BannerPlacement = "home_hero"
)

// Valid reports whether p is a placement the storefront knows.
func (p BannerPlacement) Valid() bool {
	return p == BannerPlacementHomeHero
}

// BannerTextMaxChars caps each title and link label, per language.
const BannerTextMaxChars = 120

// LocalizedText is one short storefront string in the three storefront
// languages. An empty language falls back to Korean on the storefront.
type LocalizedText struct {
	Ko string `json:"ko"`
	En string `json:"en"`
	Zh string `json:"zh"`
}

func (t LocalizedText) trimmed() LocalizedText {
	return LocalizedText{Ko: strings.TrimSpace(t.Ko), En: strings.TrimSpace(t.En), Zh: strings.TrimSpace(t.Zh)}
}

func (t LocalizedText) validate(field string) error {
	for lang, v := range map[string]string{"ko": t.Ko, "en": t.En, "zh": t.Zh} {
		if utf8.RuneCountInString(v) > BannerTextMaxChars {
			return fmt.Errorf("%s.%s is longer than %d characters", field, lang, BannerTextMaxChars)
		}
	}
	return nil
}

// Banner is an image staff put on the storefront and take down by hand.
// There is no schedule: Active is the whole switch.
type Banner struct {
	ID        string          `json:"id"`
	Placement BannerPlacement `json:"placement"`
	// ImageURL is the desktop (wide) image; MobileImageURL, when set, replaces
	// it on narrow screens.
	ImageURL       string        `json:"image_url"`
	MobileImageURL string        `json:"mobile_image_url,omitempty"`
	Title          LocalizedText `json:"title"`
	// LinkURL is where a click goes: a storefront path ("/category/…") or an
	// https URL. Empty means the banner is not a link.
	LinkURL   string        `json:"link_url,omitempty"`
	LinkLabel LocalizedText `json:"link_label"`
	Active    bool          `json:"active"`
	// SortOrder orders banners in one placement, lowest first.
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
}

// Normalize trims the free text and defaults the placement.
func (b *Banner) Normalize() {
	if b.Placement == "" {
		b.Placement = BannerPlacementHomeHero
	}
	b.ImageURL = strings.TrimSpace(b.ImageURL)
	b.MobileImageURL = strings.TrimSpace(b.MobileImageURL)
	b.LinkURL = strings.TrimSpace(b.LinkURL)
	b.Title = b.Title.trimmed()
	b.LinkLabel = b.LinkLabel.trimmed()
}

// Validate checks a banner as staff submit it.
func (b Banner) Validate() error {
	if !b.Placement.Valid() {
		return fmt.Errorf("placement %q is not one of home_hero", b.Placement)
	}
	if b.ImageURL == "" {
		return fmt.Errorf("image_url is required")
	}
	if !bannerImageURL(b.ImageURL) {
		return fmt.Errorf("image_url must be an http(s) URL or a path starting with /")
	}
	if b.MobileImageURL != "" && !bannerImageURL(b.MobileImageURL) {
		return fmt.Errorf("mobile_image_url must be an http(s) URL or a path starting with /")
	}
	if b.LinkURL != "" && !bannerLinkURL(b.LinkURL) {
		return fmt.Errorf("link_url must be a storefront path starting with / or an https URL")
	}
	if b.SortOrder < 0 {
		return fmt.Errorf("sort_order cannot be negative")
	}
	if err := b.Title.validate("title"); err != nil {
		return err
	}
	return b.LinkLabel.validate("link_label")
}

func bannerImageURL(u string) bool {
	return strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") || bannerPath(u)
}

// bannerLinkURL allows a site path or https, never javascript: or a
// protocol-relative "//host" that would leave the site unannounced.
func bannerLinkURL(u string) bool {
	return strings.HasPrefix(u, "https://") || bannerPath(u)
}

func bannerPath(u string) bool {
	return strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") && !strings.ContainsAny(u, " \\")
}
