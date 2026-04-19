package dto

// ExperienceRange is a parsed experience band with a lower bound and optional upper bound.
type ExperienceRange struct {
	Min int  // inclusive lower bound (years)
	Max *int // inclusive upper bound; nil means unbounded
}
