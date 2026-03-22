package nginx

import "testing"

// --- compareVersions tests ---

func TestCompareVersions_Equal(t *testing.T) {
	result := compareVersions("1.25.1", "1.25.1")
	if result != 0 {
		t.Errorf("compareVersions(\"1.25.1\", \"1.25.1\") = %d, want 0", result)
	}
}

func TestCompareVersions_FirstGreater(t *testing.T) {
	result := compareVersions("1.25.1", "1.24.0")
	if result != 1 {
		t.Errorf("compareVersions(\"1.25.1\", \"1.24.0\") = %d, want 1", result)
	}
}

func TestCompareVersions_SecondGreater(t *testing.T) {
	result := compareVersions("1.24.0", "1.25.1")
	if result != -1 {
		t.Errorf("compareVersions(\"1.24.0\", \"1.25.1\") = %d, want -1", result)
	}
}

func TestCompareVersions_DifferentLengths(t *testing.T) {
	result := compareVersions("1.25", "1.25.1")
	if result != -1 {
		t.Errorf("compareVersions(\"1.25\", \"1.25.1\") = %d, want -1", result)
	}
}

func TestCompareVersions_MajorDifference(t *testing.T) {
	result := compareVersions("2.0.0", "1.99.99")
	if result != 1 {
		t.Errorf("compareVersions(\"2.0.0\", \"1.99.99\") = %d, want 1", result)
	}
}

func TestCompareVersions_Empty(t *testing.T) {
	result := compareVersions("", "1.0.0")
	if result != -1 {
		t.Errorf("compareVersions(\"\", \"1.0.0\") = %d, want -1", result)
	}
}

func TestCompareVersions_NonNumeric(t *testing.T) {
	// Non-numeric parts are parsed as 0 by strconv.Atoi, so "1.x.0" behaves like "1.0.0".
	result := compareVersions("1.x.0", "1.0.0")
	if result != 0 {
		t.Errorf("compareVersions(\"1.x.0\", \"1.0.0\") = %d, want 0 (non-numeric parts parse as 0)", result)
	}
}

// --- SanitizeDomain tests ---

func TestSanitizeDomain_SimpleDomain(t *testing.T) {
	got := SanitizeDomain("example.com")
	want := "example_com"
	if got != want {
		t.Errorf("SanitizeDomain(\"example.com\") = %q, want %q", got, want)
	}
}

func TestSanitizeDomain_Subdomain(t *testing.T) {
	got := SanitizeDomain("sub.example.com")
	want := "sub_example_com"
	if got != want {
		t.Errorf("SanitizeDomain(\"sub.example.com\") = %q, want %q", got, want)
	}
}

func TestSanitizeDomain_DomainWithUnderscore(t *testing.T) {
	got := SanitizeDomain("my_app.example.com")
	want := "my__app_example_com"
	if got != want {
		t.Errorf("SanitizeDomain(\"my_app.example.com\") = %q, want %q", got, want)
	}
}

func TestSanitizeDomain_NoDotsOrUnderscores(t *testing.T) {
	got := SanitizeDomain("localhost")
	want := "localhost"
	if got != want {
		t.Errorf("SanitizeDomain(\"localhost\") = %q, want %q", got, want)
	}
}

// --- UnsanitizeDomain tests ---

func TestUnsanitizeDomain_ReverseSimple(t *testing.T) {
	got := UnsanitizeDomain("example_com")
	want := "example.com"
	if got != want {
		t.Errorf("UnsanitizeDomain(\"example_com\") = %q, want %q", got, want)
	}
}

func TestUnsanitizeDomain_ReverseUnderscore(t *testing.T) {
	got := UnsanitizeDomain("my__app_example_com")
	want := "my_app.example.com"
	if got != want {
		t.Errorf("UnsanitizeDomain(\"my__app_example_com\") = %q, want %q", got, want)
	}
}

// --- Round-trip test ---

func TestSanitizeUnsanitizeRoundTrip(t *testing.T) {
	domains := []string{
		"example.com",
		"sub.example.com",
		"my_app.example.com",
		"localhost",
		"deep.sub.domain.example.co.uk",
		"under_score.host_name.example.com",
	}

	for _, domain := range domains {
		sanitized := SanitizeDomain(domain)
		restored := UnsanitizeDomain(sanitized)
		if restored != domain {
			t.Errorf("round-trip failed for %q: SanitizeDomain -> %q -> UnsanitizeDomain -> %q",
				domain, sanitized, restored)
		}
	}
}
