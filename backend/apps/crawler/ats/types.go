package ats

// This file re-exports all types from the types/ sub-package so callers only
// need to import "github.com/trushilpatel/jobradar/apps/crawler/ats" — they never need to know about sub-packages.

import "github.com/trushilpatel/jobradar/apps/crawler/ats/types"

type (
	RawJob                  = types.RawJob
	FetchJobsItem           = types.FetchJobsItem
	FetchJobsResponse       = types.FetchJobsResponse
	FetchJobContentResponse = types.FetchJobContentResponse
	Adapter                 = types.Adapter
	ContentFetcher          = types.ContentFetcher
	KnownJobsProvider       = types.KnownJobsProvider
	StreamPublisher         = types.StreamPublisher
	SeenChecker             = types.SeenChecker
)

// StripHTML removes all HTML tags, decodes entities, and collapses whitespace.
func StripHTML(s string) string { return types.StripHTML(s) }
