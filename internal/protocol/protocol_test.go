package protocol

import "testing"

func TestIsUUIDv4AcceptsLowerCaseVersion4WithVariant10(t *testing.T) {
	// The first digit of the fourth group carries the variant: 10xx is 8, 9, a or b.
	for _, id := range []string{
		"0f8fad5b-d9cb-469f-8165-70867728950e",
		"0f8fad5b-d9cb-469f-9165-70867728950e",
		"0f8fad5b-d9cb-469f-a165-70867728950e",
		"0f8fad5b-d9cb-469f-b165-70867728950e",
	} {
		if !IsUUIDv4(id) {
			t.Errorf("IsUUIDv4(%q) = false, want true", id)
		}
	}
}

func TestIsUUIDv4RejectsOtherVersions(t *testing.T) {
	for _, id := range []string{
		"c232ab00-9414-11ec-b3c8-9f6bdeced846", // v1
		"0f8fad5b-d9cb-369f-a165-70867728950e", // v3
		"0f8fad5b-d9cb-569f-a165-70867728950e", // v5
		"0f8fad5b-d9cb-769f-a165-70867728950e", // v7
	} {
		if IsUUIDv4(id) {
			t.Errorf("IsUUIDv4(%q) = true, want false", id)
		}
	}
}

func TestIsUUIDv4RejectsOtherVariants(t *testing.T) {
	for _, id := range []string{
		"0f8fad5b-d9cb-469f-7165-70867728950e", // 0111: NCS
		"0f8fad5b-d9cb-469f-c165-70867728950e", // 1100: Microsoft
		"0f8fad5b-d9cb-469f-0165-70867728950e",
		"0f8fad5b-d9cb-469f-f165-70867728950e",
	} {
		if IsUUIDv4(id) {
			t.Errorf("IsUUIDv4(%q) = true, want false", id)
		}
	}
}

func TestIsUUIDv4RejectsUpperCase(t *testing.T) {
	for _, id := range []string{
		"0F8FAD5B-D9CB-469F-A165-70867728950E",
		"0f8fad5b-d9cb-469f-A165-70867728950e",
		"0f8fad5b-d9cb-469f-a165-70867728950E",
	} {
		if IsUUIDv4(id) {
			t.Errorf("IsUUIDv4(%q) = true, want false", id)
		}
	}
}

func TestIsUUIDv4RejectsOtherShapes(t *testing.T) {
	for _, id := range []string{
		"",
		"0f8fad5bd9cb469fa16570867728950e",
		"{0f8fad5b-d9cb-469f-a165-70867728950e}",
		"urn:uuid:0f8fad5b-d9cb-469f-a165-70867728950e",
		"0f8fad5b-d9cb-469f-a165-70867728950",
		"0f8fad5b-d9cb-469f-a165-70867728950e0",
		"0f8fad5bd-9cb-469f-a165-70867728950e",
		"0f8fad5b-d9cb-469f-a165-7086772895-e",
		"0f8fad5b-d9cb-469f-a165-70867728950g",
		"0f8fad5b-d9cb-469f-a165 70867728950e",
		" 0f8fad5b-d9cb-469f-a165-70867728950e",
	} {
		if IsUUIDv4(id) {
			t.Errorf("IsUUIDv4(%q) = true, want false", id)
		}
	}
}
