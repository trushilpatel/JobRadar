package dto

// RoleCategoryCount holds job counts per role category.
type RoleCategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}
