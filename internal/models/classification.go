package models

import (
	"sort"
	"strings"
)

// CategoryOption represents a selectable venue category.
type CategoryOption struct {
	ID    int
	Label string
}

var categoryLabels = map[int]string{
	0:  "Restaurant (Generic)",
	1:  "Health Store",
	2:  "Veg Store",
	3:  "Bakery",
	4:  "B&B",
	5:  "Delivery",
	6:  "Catering",
	7:  "Organization",
	8:  "Farmer's Market",
	10: "Food Truck",
	11: "Market Vendor",
	12: "Ice Cream",
	13: "Juice Bar",
	14: "Professional",
	15: "Coffee & Tea",
	16: "Spa",
	99: "Other",
}

// VenueTypeLabel returns the human-readable label for a venue entry type.
func VenueTypeLabel(entryType int) string {
	if entryType == 2 {
		return "Store"
	}
	return "Restaurant"
}

// VenueTypeFromLabel converts a venue type label back to its entry type ID.
func VenueTypeFromLabel(label string) int {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "store":
		return 2
	default:
		return 1
	}
}

// VeganStatusLabel returns the vegan status label derived from flags.
func VeganStatusLabel(entryType, vegOnly, vegan int) string {
	if vegOnly == 1 && vegan == 1 {
		return "Vegan"
	}
	if vegOnly == 1 && vegan == 0 {
		return "Vegetarian"
	}
	return "Veg-Options"
}

// VeganFlagsFromStatus converts a vegan status label into vegan/vegonly flag values.
func VeganFlagsFromStatus(entryType int, status string) (vegan int, vegOnly int) {
	_ = entryType // entry type does not restrict vegan flags
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "vegan":
		return 1, 1
	case "vegetarian":
		return 0, 1
	case "veg-options", "veg options":
		return 0, 0
	default:
		return 0, 0
	}
}

// CategoryLabel resolves a category ID to its display label.
func CategoryLabel(entryType, categoryID int) string {
	_ = entryType
	if label, ok := categoryLabels[categoryID]; ok {
		return label
	}
	if categoryID == 0 {
		return categoryLabels[0]
	}
	return ""
}

// CategoryIDFromLabel looks up the category ID for a given label.
func CategoryIDFromLabel(label string) int {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return 0
	}

	for id, lbl := range categoryLabels {
		if strings.EqualFold(lbl, trimmed) {
			return id
		}
	}
	return 0
}

// StoreCategoryOptions returns a sorted slice of category options for the editor dropdown.
func StoreCategoryOptions() []CategoryOption {
	ids := make([]int, 0, len(categoryLabels))
	for id := range categoryLabels {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	options := make([]CategoryOption, 0, len(ids))
	for _, id := range ids {
		options = append(options, CategoryOption{ID: id, Label: categoryLabels[id]})
	}
	return options
}
