package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultBase = "http://127.0.0.1:8080"

var baseURL = defaultBase

var client = &http.Client{
	Timeout: 120 * time.Second,
}

type docCount struct {
	total       int
	processed   int
	inReview    int
	failed      int
	assetLess   int
}

type snapshot struct {
	docs       docCount
	assets     int
	accounts   int
	movements  int
	reviews    int
}

func base() string {
	if v := os.Getenv("SEED_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return baseURL
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func logf(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
}

func get(path string) (int, string, error) {
	req, err := http.NewRequest("GET", base()+path, nil)
	if err != nil {
		return 0, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func postJSON(path string, payload interface{}) (int, string, error) {
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(payload); err != nil {
		return 0, "", err
	}
	req, err := http.NewRequest("POST", base()+path, buf)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func postRaw(path string, contentType string, body io.Reader) (int, string, error) {
	req, err := http.NewRequest("POST", base()+path, body)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), nil
}

// postMultipart uploads a single file using multipart/form-data. The body is a
// *bytes.Buffer (already-finalized multipart payload); the file part's declared
// contentType and the boundary are injected into the Content-Type header.
func postMultipart(path, uid, filename, note, contentType string, body *bytes.Buffer, boundary string) (int, string, error) {
	req, err := http.NewRequest("POST", base()+path, body)
	if err != nil {
		return 0, "", err
	}
	// The request itself is multipart/form-data (the file part's own MIME is
	// derived server-side from its extension); using the file's MIME here would
	// make r.MultipartReader() reject the body as "malformed request".
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), nil
}

func extractID(body string, key string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func countStatus(docs *docCount, status string) {
	switch status {
	case "processed":
		docs.processed++
	case "in_review":
		docs.inReview++
	case "failed":
		docs.failed++
	case "asset_less":
		docs.assetLess++
	}
}

func baseline(uid string) snapshot {
	var snap snapshot

	code, body, err := get("/api/users/" + uid + "/documents")
	if err == nil && code == 200 {
		var docs []map[string]interface{}
		if json.Unmarshal([]byte(body), &docs) == nil {
			snap.docs.total = len(docs)
			for _, d := range docs {
				if st, ok := d["status"].(string); ok {
					countStatus(&snap.docs, st)
				}
			}
		}
	} else {
		logf("  [warn] documents list: code=%v err=%v", code, err)
	}

	code, body, err = get("/api/users/" + uid + "/assets")
	if err == nil && code == 200 {
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(body), &arr) == nil {
			snap.assets = len(arr)
		}
	} else {
		logf("  [warn] assets list: code=%v err=%v", code, err)
	}

	code, body, err = get("/api/users/" + uid + "/finance/accounts")
	if err == nil && code == 200 {
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(body), &arr) == nil {
			snap.accounts = len(arr)
		}
	} else {
		logf("  [warn] accounts list: code=%v err=%v", code, err)
	}

	code, body, err = get("/api/users/" + uid + "/finance/movements")
	if err == nil && code == 200 {
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(body), &arr) == nil {
			snap.movements = len(arr)
		}
	} else {
		logf("  [warn] movements list: code=%v err=%v", code, err)
	}

	code, body, err = get("/api/users/" + uid + "/ingest/reviews")
	if err == nil && code == 200 {
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(body), &arr) == nil {
			snap.reviews = len(arr)
		}
	} else {
		logf("  [warn] reviews list: code=%v err=%v", code, err)
	}

	return snap
}

func main() {
	logf("Seed base URL: %s", base())

	// ---- BASELINE ----
	logf("\n=== BASELINE ===")
	baselineMap := map[string]snapshot{}
	users := []string{"test-user", "test-user-b"}
	for _, uid := range users {
		logf("Baseline for %s:", uid)
		baselineMap[uid] = baseline(uid)
		snap := baselineMap[uid]
		logf("  docs=%d (proc=%d,rev=%d,fail=%d,assetless=%d) assets=%d accounts=%d movements=%d reviews=%d",
			snap.docs.total, snap.docs.processed, snap.docs.inReview, snap.docs.failed, snap.docs.assetLess,
			snap.assets, snap.accounts, snap.movements, snap.reviews)
	}

	// ---- UPLOAD HELPERS ----
	var dupReport []string
	var noDocReport []string

	upload := func(uid, file, note string) {
		abs, _ := filepath.Abs(file)
		name := filepath.Base(abs)
		f, err := os.Open(abs)
		if err != nil {
			logf("  [%s] OPEN ERR %v", name, err)
			return
		}
		defer f.Close()

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", name)
		if err != nil {
			logf("  [%s] multipart err %v", name, err)
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			logf("  [%s] copy err %v", name, err)
			return
		}
		writer.WriteField("note", note)
		writer.Close()

		ctype := "application/octet-stream"
		lc := strings.ToLower(name)
		if strings.HasSuffix(lc, ".pdf") {
			ctype = "application/pdf"
		} else if strings.HasSuffix(lc, ".jpg") || strings.HasSuffix(lc, ".jpeg") {
			ctype = "image/jpeg"
		} else if strings.HasSuffix(lc, ".png") {
			ctype = "image/png"
		}

		path := "/api/users/" + uid + "/documents"
		code, respBody, err := postMultipart(path, uid, name, note, ctype, body, writer.Boundary())
		if err != nil {
			logf("  [%s] HTTP ERR %v", name, err)
			return
		}
		logf("  [%s] -> %d (note: %q)", name, code, note)
		logf("       body: %s", trunc(respBody, 300))

		switch {
		case code == 201:
			id := extractID(respBody, "id")
			logf("       => created asset id=%s", id)
		case code == 202:
			id := extractID(respBody, "id")
			logf("       => held for review id=%s", id)
		case code == 409:
			var rep struct {
				ExistingAssetID         string `json:"existing_asset_id"`
				ExistingDocumentID      string `json:"existing_document_id"`
				ExistingSourceFilename  string `json:"existing_source_filename"`
			}
			if json.Unmarshal([]byte(respBody), &rep) == nil {
				dupReport = append(dupReport, fmt.Sprintf("%s -> asset %s (doc %s, file %q)", name, rep.ExistingAssetID, rep.ExistingDocumentID, rep.ExistingSourceFilename))
			} else {
				dupReport = append(dupReport, fmt.Sprintf("%s -> duplicate (body: %s)", name, trunc(respBody, 120)))
			}
			logf("       => duplicate recorded")
		case code == 422 || code == 502:
			var rep struct {
				Error string `json:"error"`
			}
			msg := respBody
			if json.Unmarshal([]byte(respBody), &rep) == nil && rep.Error != "" {
				msg = rep.Error
			}
			noDocReport = append(noDocReport, fmt.Sprintf("%s (status %d): %s", name, code, strings.TrimSpace(msg)))
			logf("       => no-doc outcome recorded")
		default:
			logf("       => UNHANDLED status %d", code)
		}
	}

	// ---- UPLOADS for test-user ----
	logf("\n=== UPLOADS test-user ===")
	upload("test-user", "C:\\Projects\\procrastinator\\sample-data\\INVNAG2302754.pdf", "this is a laptop (MacBook) invoice")
	upload("test-user", "C:\\Projects\\procrastinator\\sample-data\\Annual Maintenance Contract.pdf", "annual maintenance contract for a washing machine")
	upload("test-user", "C:\\Projects\\procrastinator\\sample-data\\IMG_20260905_133941697_HDR.jpg", "photo of a smartphone invoice")
	upload("test-user", "C:\\Projects\\procrastinator\\sample-data\\IMG_20260905_133830011_HDR.jpg", "photo of a headphones retail box")
	upload("test-user", "C:\\Projects\\procrastinator\\sample-data\\IMG_20260905_124846997_HDR.jpg", "photo of a laptop shipping label")
	upload("test-user", "C:\\Projects\\procrastinator\\sample-data\\IMG_20260905_122605809.jpg", "photo of a retail receipt for office supplies")

	// ---- UPLOADS for test-user-b ----
	logf("\n=== UPLOADS test-user-b ===")
	upload("test-user-b", "C:\\Projects\\procrastinator\\sample-data\\INVNAG2302754.pdf", "this is a laptop (MacBook) invoice")
	upload("test-user-b", "C:\\Projects\\procrastinator\\sample-data\\IMG_20260905_133941697_HDR.jpg", "photo of a smartphone invoice")

	// ---- ACCOUNTS for test-user ----
	logf("\n=== ACCOUNTS test-user ===")
	accountIDs := map[string]string{}
	accounts := []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Cur  string `json:"currency"`
	}{
		{"HDFC Savings", "bank", "INR"},
		{"Cash Wallet", "cash", "INR"},
		{"Amazon Pay", "wallet", "INR"},
	}
	for _, ac := range accounts {
		code, body, err := postJSON("/api/users/test-user/finance/accounts", ac)
		if err != nil {
			logf("  ACCOUNT ERR %v", err)
			continue
		}
		id := extractID(body, "id")
		accountIDs[ac.Name] = id
		logf("  account %s/%s/%s -> %d id=%s", ac.Name, ac.Type, ac.Cur, code, id)
	}

	// ---- MOVEMENTS for test-user ----
	logf("\n=== MOVEMENTS test-user ===")
	movementIDs := []string{}
	mkIncome := func(dest, amount, desc string) map[string]interface{} {
		return map[string]interface{}{
			"kind": "income", "amount": amount, "currency": "INR",
			"occurred_on": "2026-08-01", "description": desc,
			"destination_account_id": dest,
		}
	}
	mkExpense := func(src, amount, desc, date string) map[string]interface{} {
		return map[string]interface{}{
			"kind": "expense", "amount": amount, "currency": "INR",
			"occurred_on": date, "description": desc,
			"source_account_id": src,
		}
	}
	mkTransfer := func(src, dst, amount, desc string) map[string]interface{} {
		return map[string]interface{}{
			"kind": "transfer", "amount": amount, "currency": "INR",
			"occurred_on": "2026-08-25", "description": desc,
			"source_account_id": src, "destination_account_id": dst,
		}
	}

	movements := []struct {
		payload map[string]interface{}
		label   string
	}{
		{mkIncome(accountIDs["HDFC Savings"], "85000.00", "Salary credit"), "income salary"},
		{mkExpense(accountIDs["HDFC Savings"], "62000.00", "MacBook Pro 14", "2026-08-05"), "expense macbook"},
		{mkExpense(accountIDs["Amazon Pay"], "3499.00", "Sony WH-1000XM5 headphones", "2026-08-12"), "expense headphones"},
		{mkExpense(accountIDs["HDFC Savings"], "2100.00", "Electricity bill", "2026-08-20"), "expense electricity"},
		{mkTransfer(accountIDs["HDFC Savings"], accountIDs["Amazon Pay"], "5000.00", "Top up wallet"), "transfer topup"},
	}
	for _, mv := range movements {
		code, body, err := postJSON("/api/users/test-user/finance/movements", mv.payload)
		if err != nil {
			logf("  MOVEMENT ERR %v", err)
			continue
		}
		id := extractID(body, "id")
		if code == 201 {
			movementIDs = append(movementIDs, id)
		} else if code == 400 {
			logf("  [%s] 400 validation: %s", mv.label, trunc(body, 200))
		}
		logf("  [%s] -> %d id=%s", mv.label, code, id)
	}

	// ---- OPTIONAL asset_less best-effort ----
	logf("\n=== OPTIONAL asset_less (best-effort) ===")
	code, body, err := get("/api/users/test-user/documents?status=processed")
	if err == nil && code == 200 {
		var docs []map[string]interface{}
		if json.Unmarshal([]byte(body), &docs) == nil && len(docs) > 0 {
			firstID, _ := docs[0]["id"].(string)
			if firstID != "" {
				rebody := `{"comment":"treat this strictly as a bank statement CSV, not an asset"}`
				rcode, rresp, rerr := postRaw("/api/users/test-user/documents/"+firstID+"/reprocess", "application/json", strings.NewReader(rebody))
				if rerr == nil {
					logf("  reprocess doc=%s -> %d body: %s", firstID, rcode, trunc(rresp, 300))
				} else {
					logf("  reprocess err %v", rerr)
				}
			} else {
				logf("  no processed doc has an id field")
			}
		} else {
			logf("  no processed documents to attempt asset_less on (skipping)")
		}
	} else {
		logf("  could not list processed docs: code=%v err=%v", code, err)
	}

	// ---- VERIFY / FINAL COUNTS ----
	logf("\n=== FINAL COUNTS ===")
	final := map[string]snapshot{}
	for _, uid := range users {
		logf("Final for %s:", uid)
		final[uid] = baseline(uid)
		snap := final[uid]
		logf("  docs=%d (proc=%d,rev=%d,fail=%d,assetless=%d) assets=%d accounts=%d movements=%d reviews=%d",
			snap.docs.total, snap.docs.processed, snap.docs.inReview, snap.docs.failed, snap.docs.assetLess,
			snap.assets, snap.accounts, snap.movements, snap.reviews)
	}

	logf("\n=== 409 DUPLICATES ===")
	if len(dupReport) == 0 {
		logf("  (none)")
	} else {
		for _, d := range dupReport {
			logf("  %s", d)
		}
	}
	logf("\n=== 422/502 NO-DOC OUTCOMES ===")
	if len(noDocReport) == 0 {
		logf("  (none)")
	} else {
		for _, n := range noDocReport {
			logf("  %s", n)
		}
	}

	logf("\nSeed run complete. accounts: %v movements recorded: %d duplicates: %d nodoc: %d",
		accountIDs, len(movementIDs), len(dupReport), len(noDocReport))
}
