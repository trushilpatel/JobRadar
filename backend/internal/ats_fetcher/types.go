package ats_fetcher

// This file re-exports all types from the types/ sub-package so callers only
// need to import "internal/ats_fetcher" — they never need to know about sub-packages.

import "github.com/trushilpatel/jobradar/internal/ats_fetcher/types"

type (
	RawJob                  = types.RawJob
	FetchJobsItem           = types.FetchJobsItem
	FetchJobsResponse       = types.FetchJobsResponse
	FetchJobContentResponse = types.FetchJobContentResponse
	Adapter                 = types.Adapter
	ContentFetcher          = types.ContentFetcher
)

// StripHTML removes all HTML tags, decodes entities, and collapses whitespace.
func StripHTML(s string) string { return types.StripHTML(s) }
