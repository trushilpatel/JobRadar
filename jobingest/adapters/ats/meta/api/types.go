// Package api holds the raw JSON-LD types decoded from Meta's detail pages.
// These types mirror the schema.org JobPosting embedded inline as
// <script type="application/ld+json"> on every Meta Careers detail page.
package api

import (
	"encoding/json"
	"strings"
)

// JobPosting mirrors the schema.org JobPosting payload Meta embeds in every
// detail page. Verified across a 21-job sample drawn from the full sitemap.
//
// jobLocation may be a single object or an array — the custom jobLocations
// unmarshaller normalises both into a []JobLocation.
type JobPosting struct {
	Context          string             `json:"@context,omitempty"`
	Type             string             `json:"@type,omitempty"`
	Title            string             `json:"title,omitempty"`
	Description      string             `json:"description,omitempty"`
	Responsibilities string             `json:"responsibilities,omitempty"`
	Qualifications   string             `json:"qualifications,omitempty"`
	EmploymentType   string             `json:"employmentType,omitempty"`
	DatePosted       string             `json:"datePosted,omitempty"`
	ValidThrough     string             `json:"validThrough,omitempty"`
	DirectApply      bool               `json:"directApply,omitempty"`
	HiringOrg        HiringOrganization `json:"hiringOrganization,omitempty"`
	JobLocation      JobLocations       `json:"jobLocation,omitempty"`

	// Optional fields — rarely populated in the wild but kept for completeness.
	Industry             FlexString      `json:"industry,omitempty"`
	OccupationalCategory FlexString      `json:"occupationalCategory,omitempty"`
	BaseSalary           json.RawMessage `json:"baseSalary,omitempty"`
	Skills               FlexString      `json:"skills,omitempty"`
	JobBenefits          FlexString      `json:"jobBenefits,omitempty"`
}

// HiringOrganization holds the schema.org Organization block.
type HiringOrganization struct {
	Type string `json:"@type,omitempty"`
	Name string `json:"name,omitempty"`
	Logo string `json:"logo,omitempty"`
	URL  string `json:"sameAs,omitempty"`
}

// JobLocations handles JSON-LD's quirk: `jobLocation` may be a single object
// or an array of them. Normalises to a slice internally.
type JobLocations []JobLocation

func (jl *JobLocations) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*jl = nil
		return nil
	}
	if b[0] == '[' {
		var arr []JobLocation
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*jl = arr
		return nil
	}
	var single JobLocation
	if err := json.Unmarshal(b, &single); err != nil {
		return err
	}
	*jl = []JobLocation{single}
	return nil
}

// JobLocation mirrors schema.org Place.
type JobLocation struct {
	Type    string  `json:"@type,omitempty"`
	Address Address `json:"address"`
	Name    string  `json:"name,omitempty"`
}

// Address mirrors schema.org PostalAddress.
type Address struct {
	Locality FlexString `json:"addressLocality,omitempty"`
	Region   FlexString `json:"addressRegion,omitempty"`
	Country  FlexString `json:"addressCountry,omitempty"`
}

// FlexString decodes any of the string-shaped variants Meta emits in
// schema.org address fields:
//   - plain string            ("USA")
//   - object with .name       ({"@type":"Country","name":"USA"})
//   - array of any of above   (["USA","USA",…])
//   - object whose .name is itself a list (real production case)
//
// Recurses through containers and dedupes terminal values, joining with ", ".
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	// Plain string — overwhelmingly the common case.
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = FlexString(s)
		return nil
	}
	// Object with a "name" field — recurse into name.
	var d map[string]json.RawMessage
	if err := json.Unmarshal(b, &d); err == nil {
		if name, ok := d["name"]; ok {
			var inner FlexString
			if err := inner.UnmarshalJSON(name); err == nil {
				*f = inner
				return nil
			}
		}
		*f = ""
		return nil
	}
	// Array — recurse on each element, dedupe.
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err == nil {
		seen := map[string]struct{}{}
		var parts []string
		for _, item := range arr {
			var inner FlexString
			if err := inner.UnmarshalJSON(item); err == nil {
				str := string(inner)
				if str == "" {
					continue
				}
				if _, dup := seen[str]; dup {
					continue
				}
				seen[str] = struct{}{}
				parts = append(parts, str)
			}
		}
		*f = FlexString(strings.Join(parts, ", "))
		return nil
	}
	// Unknown shape — fall through silently.
	*f = ""
	return nil
}

func (f FlexString) String() string { return string(f) }
