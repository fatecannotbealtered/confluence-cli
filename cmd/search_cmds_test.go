package cmd

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// ─── buildCQL (pure compiler) ───────────────────────────────────────────────

func TestBuildCQL_FlagCombination(t *testing.T) {
	cql, err := buildCQL(searchFilters{
		contentType: "page",
		spaces:      []string{"ENG", "OPS"},
		title:       "roadmap",
		labels:      []string{"adr", "q3"},
		sort:        "modified",
		desc:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		`type = "page"`,
		`space in ("ENG", "OPS")`,
		`title ~ "roadmap"`,
		`label = "adr"`,
		`label = "q3"`,
		"order by lastmodified desc",
	} {
		if !strings.Contains(cql, want) {
			t.Fatalf("cql %q missing %q", cql, want)
		}
	}
	if strings.Count(cql, " AND ") != 4 {
		t.Fatalf("expected 5 clauses ANDed, got %q", cql)
	}
}

func TestBuildCQL_FlagPlusRaw(t *testing.T) {
	cql, err := buildCQL(searchFilters{
		raw:         []string{`creator = "jdoe"`},
		contentType: "blogpost",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cql, `(creator = "jdoe")`) || !strings.Contains(cql, `type = "blogpost"`) {
		t.Fatalf("cql=%q", cql)
	}
	if !strings.Contains(cql, " AND ") {
		t.Fatalf("raw and flag should be ANDed: %q", cql)
	}
}

func TestBuildCQL_RelativeDate(t *testing.T) {
	cql, err := buildCQL(searchFilters{modifiedSince: "-7d", createdUntil: "2026-01-01"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cql, `lastmodified >= now("-7d")`) {
		t.Fatalf("relative date not compiled: %q", cql)
	}
	if !strings.Contains(cql, `created <= "2026-01-01"`) {
		t.Fatalf("absolute date not compiled: %q", cql)
	}
}

func TestBuildCQL_InvalidDate(t *testing.T) {
	if _, err := buildCQL(searchFilters{modifiedSince: "yesterday"}); err == nil {
		t.Fatal("expected error for invalid date")
	}
}

func TestBuildCQL_InvalidType(t *testing.T) {
	if _, err := buildCQL(searchFilters{contentType: "widget"}); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestBuildCQL_EmptyQuery(t *testing.T) {
	if _, err := buildCQL(searchFilters{}); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestBuildCQL_SortConflict(t *testing.T) {
	if _, err := buildCQL(searchFilters{title: "x", sort: "created", desc: true, asc: true}); err == nil {
		t.Fatal("expected error for --desc + --asc")
	}
}

// ─── search command ─────────────────────────────────────────────────────────

func searchHandler(t *testing.T, capture *string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/user/current" {
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
			return
		}
		if r.URL.Path == "/rest/api/search" {
			if capture != nil {
				*capture = r.URL.Query().Get("cql")
			}
			_, _ = w.Write([]byte(`{"results":[{"title":"Roadmap","excerpt":"the <b>roadmap</b>","url":"/x/1","lastModified":"2026-06-01","content":{"id":"101","type":"page","space":{"key":"ENG"},"_links":{"webui":"/display/ENG/Roadmap"}}}],"start":0,"limit":25,"size":1,"totalSize":1,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestSearch_HappyPath(t *testing.T) {
	var cql string
	mockConfluenceServer(t, searchHandler(t, &cql))
	stdout, _ := runRootOK(t, "search", "--type", "page", "--space", "ENG", "--title", "roadmap")
	var data struct {
		Results []struct {
			ID        string   `json:"id"`
			Type      string   `json:"type"`
			SpaceKey  string   `json:"space_key"`
			URL       string   `json:"url"`
			Untrusted []string `json:"_untrusted"`
		} `json:"items"`
		TotalSize int `json:"total_size"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Results) != 1 || data.Results[0].ID != "101" || data.Results[0].SpaceKey != "ENG" {
		t.Fatalf("results=%+v", data.Results)
	}
	if !strings.HasSuffix(data.Results[0].URL, "/display/ENG/Roadmap") {
		t.Fatalf("web url not assembled: %q", data.Results[0].URL)
	}
	if len(data.Results[0].Untrusted) != 2 {
		t.Fatalf("expected title+excerpt untrusted, got %v", data.Results[0].Untrusted)
	}
	if !strings.Contains(cql, `type = "page"`) {
		t.Fatalf("server cql=%q", cql)
	}
}

func TestSearch_CountOnly(t *testing.T) {
	mockConfluenceServer(t, searchHandler(t, nil))
	stdout, _ := runRootOK(t, "search", "--text", "roadmap", "--count-only")
	var data struct {
		TotalSize int `json:"total_size"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.TotalSize != 1 {
		t.Fatalf("total_size=%d", data.TotalSize)
	}
}

func TestSearch_InvalidDateUsageError(t *testing.T) {
	mockConfluenceServer(t, searchHandler(t, nil))
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "search", "--modified-since", "lastweek")
	errPayload := decodeEnvelopeError(t, stdout)
	if errPayload["code"] != "E_VALIDATION" {
		t.Fatalf("error=%v", errPayload)
	}
}

func TestSearch_ServerCQL400PassesThrough(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/user/current" {
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"statusCode":400,"message":"Could not parse cql : bad token"}`))
	})
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "search", `bad cql (`)
	errPayload := decodeEnvelopeError(t, stdout)
	if errPayload["code"] != "E_USAGE" {
		t.Fatalf("error code=%v", errPayload["code"])
	}
	details, _ := errPayload["details"].(map[string]any)
	if details == nil || !strings.Contains(details["server_message"].(string), "Could not parse cql") {
		t.Fatalf("server_message not passed through: %v", errPayload)
	}
}

func TestSearch_AllPaginatesAndCaps(t *testing.T) {
	// Server always reports has_more, so --all must stop at the cap and notice.
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/user/current" {
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
			return
		}
		start, _ := parseStart(r.URL.Query())
		results := make([]string, 0, 100)
		for i := 0; i < 100; i++ {
			results = append(results, `{"title":"t","excerpt":"e","url":"/x","content":{"id":"`+itoa(start+i)+`","type":"page"}}`)
		}
		_, _ = w.Write([]byte(`{"results":[` + strings.Join(results, ",") + `],"start":` + itoa(start) + `,"limit":100,"size":100,"totalSize":5000,"_links":{"next":"/next"}}`))
	})
	stdout, _ := runRootOK(t, "search", "--all", "--text", "x")
	env := decodeEnvelope(t, stdout)
	var data struct {
		Results []any `json:"items"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Results) != searchAllCap {
		t.Fatalf("expected cap %d results, got %d", searchAllCap, len(data.Results))
	}
	meta, _ := env["meta"].(map[string]any)
	notices, _ := meta["notices"].([]any)
	if len(notices) == 0 {
		t.Fatalf("expected cap notice in meta.notices: %v", meta)
	}
}

func parseStart(q url.Values) (int, error) {
	s := q.Get("start")
	if s == "" {
		return 0, nil
	}
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestSearch_ExcerptHighlightMarkersStripped(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"content":{"id":"1","type":"page","space":{"key":"DEV"},"_links":{"webui":"/x"}},"title":"T","excerpt":"the @@@hl@@@roadmap@@@endhl@@@ plan","url":"/x","entityType":"content","lastModified":"2026-06-01T00:00:00.000Z"}],"start":0,"limit":25,"size":1,"totalSize":1,"_links":{}}`))
	})
	stdout, _ := runRootOK(t, "search", "--text", "roadmap", "--compact")
	var data struct {
		Results []struct {
			Excerpt string `json:"excerpt"`
		} `json:"items"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Results) != 1 {
		t.Fatalf("results=%d", len(data.Results))
	}
	ex := data.Results[0].Excerpt
	if strings.Contains(ex, "@@@") {
		t.Fatalf("highlight markers not stripped: %q", ex)
	}
	if !strings.Contains(ex, "roadmap") {
		t.Fatalf("matched text lost: %q", ex)
	}
}
