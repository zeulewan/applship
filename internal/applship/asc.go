package applship

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const ascBaseURL = "https://api.appstoreconnect.apple.com"

type ASCClient struct {
	KeyPath  string
	KeyID    string
	IssuerID string
	Client   *http.Client
}

type ascResponse struct {
	Data     json.RawMessage   `json:"data"`
	Included []json.RawMessage `json:"included"`
	Errors   []ascError        `json:"errors"`
}

type ascError struct {
	Status string `json:"status"`
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type ascResource struct {
	Type          string         `json:"type"`
	ID            string         `json:"id"`
	Attributes    map[string]any `json:"attributes"`
	Relationships map[string]any `json:"relationships"`
}

func NewASCClientFromEnv() (*ASCClient, error) {
	c := &ASCClient{
		KeyPath:  os.Getenv("APP_STORE_CONNECT_KEY_PATH"),
		KeyID:    os.Getenv("APP_STORE_CONNECT_KEY_ID"),
		IssuerID: os.Getenv("APP_STORE_CONNECT_ISSUER_ID"),
		Client:   &http.Client{Timeout: 60 * time.Second},
	}
	if c.KeyPath == "" || c.KeyID == "" || c.IssuerID == "" {
		return nil, errors.New("set APP_STORE_CONNECT_KEY_PATH, APP_STORE_CONNECT_KEY_ID, and APP_STORE_CONNECT_ISSUER_ID")
	}
	return c, nil
}

func (c *ASCClient) token() (string, error) {
	keyBytes, err := os.ReadFile(filepath.Clean(c.KeyPath))
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return "", errors.New("invalid App Store Connect private key PEM")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	key, ok := keyAny.(*ecdsa.PrivateKey)
	if !ok {
		return "", errors.New("App Store Connect key is not ECDSA")
	}

	now := time.Now()
	header := map[string]string{"alg": "ES256", "kid": c.KeyID, "typ": "JWT"}
	payload := map[string]any{
		"iss": c.IssuerID,
		"iat": now.Unix(),
		"exp": now.Add(20 * time.Minute).Unix(),
		"aud": "appstoreconnect-v1",
	}
	headerBytes, _ := json.Marshal(header)
	payloadBytes, _ := json.Marshal(payload)
	signingInput := b64url(headerBytes) + "." + b64url(payloadBytes)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	signature := append(pad32(r), pad32(s)...)
	return signingInput + "." + b64url(signature), nil
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func pad32(n *big.Int) []byte {
	out := make([]byte, 32)
	nb := n.Bytes()
	copy(out[32-len(nb):], nb)
	return out
}

func (c *ASCClient) Request(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	token, err := c.token()
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, ascBaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d\n%s", method, path, resp.StatusCode, string(respBody))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

func (c *ASCClient) FindAppID(bundleID string) (string, error) {
	var resp struct {
		Data []ascResource `json:"data"`
	}
	path := "/v1/apps?" + url.Values{"filter[bundleId]": {bundleID}}.Encode()
	if err := c.Request("GET", path, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("no App Store Connect app found for bundle id %s", bundleID)
	}
	return resp.Data[0].ID, nil
}

func (c *ASCClient) LookupAppID(bundleID string) (string, error) {
	var resp struct {
		Data []ascResource `json:"data"`
	}
	path := "/v1/apps?" + url.Values{"filter[bundleId]": {bundleID}}.Encode()
	if err := c.Request("GET", path, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", nil
	}
	return resp.Data[0].ID, nil
}

func (c *ASCClient) FindBundleID(identifier string) (string, error) {
	var resp struct {
		Data []ascResource `json:"data"`
	}
	path := "/v1/bundleIds?" + url.Values{"filter[identifier]": {identifier}}.Encode()
	if err := c.Request("GET", path, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", nil
	}
	return resp.Data[0].ID, nil
}

func (c *ASCClient) CreateBundleID(name, identifier string) (string, error) {
	payload := map[string]any{"data": map[string]any{
		"type":       "bundleIds",
		"attributes": map[string]string{"name": name, "identifier": identifier, "platform": "IOS"},
	}}
	var created struct {
		Data ascResource `json:"data"`
	}
	if err := c.Request("POST", "/v1/bundleIds", payload, &created); err != nil {
		return "", err
	}
	return created.Data.ID, nil
}

func (c *ASCClient) EnsureBundleID(name, identifier string) (string, error) {
	id, err := c.FindBundleID(identifier)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	return c.CreateBundleID(name, identifier)
}

func (c *ASCClient) CreateApp(name, bundleResourceID, sku, primaryLocale string) (string, error) {
	if primaryLocale == "" {
		primaryLocale = "en-US"
	}
	if sku == "" {
		sku = bundleResourceID
	}
	payload := map[string]any{"data": map[string]any{
		"type": "apps",
		"attributes": map[string]string{
			"name":          name,
			"sku":           sku,
			"primaryLocale": primaryLocale,
			"platform":      "IOS",
		},
		"relationships": map[string]any{
			"bundleId": map[string]any{"data": map[string]string{"type": "bundleIds", "id": bundleResourceID}},
		},
	}}
	var created struct {
		Data ascResource `json:"data"`
	}
	if err := c.Request("POST", "/v1/apps", payload, &created); err != nil {
		return "", err
	}
	return created.Data.ID, nil
}

func (c *ASCClient) GetOrCreateVersion(appID, version string) (string, error) {
	var resp struct {
		Data []ascResource `json:"data"`
	}
	if err := c.Request("GET", "/v1/apps/"+appID+"/appStoreVersions", nil, &resp); err != nil {
		return "", err
	}
	for _, item := range resp.Data {
		if item.Attributes["platform"] == "IOS" && item.Attributes["versionString"] == version {
			return item.ID, nil
		}
	}
	payload := map[string]any{"data": map[string]any{
		"type":       "appStoreVersions",
		"attributes": map[string]any{"platform": "IOS", "versionString": version},
		"relationships": map[string]any{
			"app": map[string]any{"data": map[string]string{"type": "apps", "id": appID}},
		},
	}}
	var created struct {
		Data ascResource `json:"data"`
	}
	if err := c.Request("POST", "/v1/appStoreVersions", payload, &created); err != nil {
		return "", err
	}
	return created.Data.ID, nil
}

func (c *ASCClient) LatestEligibleBuild(appID, buildNumber string) (string, error) {
	values := url.Values{"filter[app]": {appID}, "sort": {"-uploadedDate"}, "limit": {"50"}}
	if buildNumber != "" {
		values.Set("filter[version]", buildNumber)
	}
	var resp struct {
		Data []ascResource `json:"data"`
	}
	if err := c.Request("GET", "/v1/builds?"+values.Encode(), nil, &resp); err != nil {
		return "", err
	}
	for _, build := range resp.Data {
		if build.Attributes["processingState"] == "VALID" && build.Attributes["buildAudienceType"] == "APP_STORE_ELIGIBLE" {
			return build.ID, nil
		}
	}
	return "", errors.New("no VALID App Store-eligible build found")
}

func (c *ASCClient) AttachBuild(versionID, buildID string) error {
	payload := map[string]any{"data": map[string]string{"type": "builds", "id": buildID}}
	return c.Request("PATCH", "/v1/appStoreVersions/"+versionID+"/relationships/build", payload, nil)
}

func (c *ASCClient) UpdateWhatsNew(versionID, whatsNew string) error {
	var locs struct {
		Data []ascResource `json:"data"`
	}
	if err := c.Request("GET", "/v1/appStoreVersions/"+versionID+"/appStoreVersionLocalizations", nil, &locs); err != nil {
		return err
	}
	locID := ""
	for _, loc := range locs.Data {
		if loc.Attributes["locale"] == "en-US" {
			locID = loc.ID
			break
		}
	}
	if locID == "" {
		payload := map[string]any{"data": map[string]any{
			"type":       "appStoreVersionLocalizations",
			"attributes": map[string]string{"locale": "en-US"},
			"relationships": map[string]any{
				"appStoreVersion": map[string]any{"data": map[string]string{"type": "appStoreVersions", "id": versionID}},
			},
		}}
		var created struct {
			Data ascResource `json:"data"`
		}
		if err := c.Request("POST", "/v1/appStoreVersionLocalizations", payload, &created); err != nil {
			return err
		}
		locID = created.Data.ID
	}
	payload := map[string]any{"data": map[string]any{
		"type":       "appStoreVersionLocalizations",
		"id":         locID,
		"attributes": map[string]string{"whatsNew": whatsNew},
	}}
	return c.Request("PATCH", "/v1/appStoreVersionLocalizations/"+locID, payload, nil)
}

func (c *ASCClient) Submit(appID, versionID string) (string, error) {
	payload := map[string]any{"data": map[string]any{
		"type": "reviewSubmissions",
		"relationships": map[string]any{
			"app": map[string]any{"data": map[string]string{"type": "apps", "id": appID}},
		},
	}}
	var created struct {
		Data ascResource `json:"data"`
	}
	if err := c.Request("POST", "/v1/reviewSubmissions", payload, &created); err != nil {
		return "", err
	}
	submissionID := created.Data.ID
	item := map[string]any{"data": map[string]any{
		"type": "reviewSubmissionItems",
		"relationships": map[string]any{
			"reviewSubmission": map[string]any{"data": map[string]string{"type": "reviewSubmissions", "id": submissionID}},
			"appStoreVersion":  map[string]any{"data": map[string]string{"type": "appStoreVersions", "id": versionID}},
		},
	}}
	if err := c.Request("POST", "/v1/reviewSubmissionItems", item, nil); err != nil {
		return "", err
	}
	submitPayload := map[string]any{"data": map[string]any{
		"type":       "reviewSubmissions",
		"id":         submissionID,
		"attributes": map[string]bool{"submitted": true},
	}}
	var result struct {
		Data ascResource `json:"data"`
	}
	if err := c.Request("PATCH", "/v1/reviewSubmissions/"+submissionID, submitPayload, &result); err != nil {
		return "", err
	}
	state, _ := result.Data.Attributes["state"].(string)
	return state, nil
}

func (c *ASCClient) PrintStatus(bundleID string) error {
	appID, err := c.FindAppID(bundleID)
	if err != nil {
		return err
	}
	var versions struct {
		Data []ascResource `json:"data"`
	}
	if err := c.Request("GET", "/v1/apps/"+appID+"/appStoreVersions", nil, &versions); err != nil {
		return err
	}
	fmt.Println("Versions:")
	for _, v := range versions.Data {
		fmt.Printf("  %s  %s  %s\n", v.Attributes["versionString"], v.Attributes["platform"], v.Attributes["appStoreState"])
	}
	var builds struct {
		Data []ascResource `json:"data"`
	}
	values := url.Values{"filter[app]": {appID}, "sort": {"-uploadedDate"}, "limit": {"10"}}
	if err := c.Request("GET", "/v1/builds?"+values.Encode(), nil, &builds); err != nil {
		return err
	}
	fmt.Println("Builds:")
	for _, b := range builds.Data {
		fmt.Printf("  %s  %s  %s  %s\n", b.Attributes["version"], b.Attributes["processingState"], b.Attributes["buildAudienceType"], b.Attributes["uploadedDate"])
	}
	return nil
}
