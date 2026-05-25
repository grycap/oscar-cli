/*
Copyright (C) GRyCAP - I3M - UPV

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cluster

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/grycap/oscar/v4/pkg/types"
	"github.com/indigo-dc/liboidcagent-go"
)

const infoPath = "/system/info"
const configPath = "/system/config"
const statusPath = "/system/status"
const _DEFAULT_TIMEOUT = 20
const BASIC_AUTH = "*cluster.basicAuthRoundTripper"

var (
	// ErrParsingEndpoint error message for cluster endpoint parsing
	ErrParsingEndpoint = errors.New("error parsing the cluster endpoint, please check that you have typed it correctly")
	// ErrMakingRequest error message for making requests
	ErrMakingRequest = errors.New("error making the request")
	// ErrSendingRequest error message for sending requests
	ErrSendingRequest = errors.New("unable to communicate with the cluster, please check that the endpoint is well typed and accessible")
)

type RefreshToken struct {
	Exp          int    `json:"exp"`
	Iat          int    `json:"iat"`
	Jti          string `json:"jti"`
	Iss          string `json:"iss"`
	Aud          string `json:"aud"`
	Sub          string `json:"sub"`
	Typ          string `json:"typ"`
	Azp          string `json:"azp"`
	Nonce        string `json:"nonce"`
	SessionState string `json:"session_state"`
	Scope        string `json:"scope"`
	Sid          string `json:"sid"`
}

type ResponseRefreshToken struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
	IdToken          string `json:"id_token"`
	NotBeforePolicy  int    `json:"not-before-policy"`
	SessionState     string `json:"session_state"`
	Scope            string `json:"scope"`
}

// Cluster defines the configuration of an OSCAR cluster
type Cluster struct {
	Endpoint         string `json:"endpoint"`
	AuthUser         string `json:"auth_user,omitempty"`
	AuthPassword     string `json:"auth_password,omitempty"`
	OIDCAccountName  string `json:"oidc_account_name,omitempty"`
	OIDCRefreshToken string `json:"oidc_refresh_token,omitempty"`
	SSLVerify        bool   `json:"ssl_verify"`
	Memory           string `json:"memory"`
	LogLevel         string `json:"log_level"`
}

type basicAuthRoundTripper struct {
	username  string
	password  string
	transport http.RoundTripper
}

type tokenRoundTripper struct {
	token     string
	transport http.RoundTripper
}

// RoundTrip function to implement the RoundTripper interface adding basic auth headers
func (bart *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add basic auth to requests
	req.SetBasicAuth(bart.username, bart.password)
	return bart.transport.RoundTrip(req)
}

// RoundTrip function to implement the RoundTripper interface adding a bearer token
func (trt *tokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add bearer token to requests
	req.Header.Add("Authorization", "Bearer "+trt.token)
	return trt.transport.RoundTrip(req)
}

func (cluster *Cluster) SetToken(client *http.Client, token string) {
	var transport http.RoundTripper = &http.Transport{
		// Enable/disable ssl verification
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !cluster.SSLVerify},
	}
	client.Transport = &tokenRoundTripper{
		token:     token,
		transport: transport,
	}
}

// GetClientSafe returns an HTTP client to communicate with the cluster without exiting on errors.
func (cluster *Cluster) GetClientSafe(args ...int) (*http.Client, error) {
	timeout := _DEFAULT_TIMEOUT

	var transport http.RoundTripper = &http.Transport{
		// Enable/disable ssl verification
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !cluster.SSLVerify},
	}

	if cluster.OIDCAccountName != "" {
		// Get token from OIDC Agent
		token, err := liboidcagent.GetAccessToken(liboidcagent.TokenRequest{
			ShortName:       cluster.OIDCAccountName,
			MinValidPeriod:  600,
			Scopes:          []string{},
			ApplicationHint: "OSCAR-CLI",
		})

		if err != nil {
			return nil, fmt.Errorf("unable to get the OIDC token, please check your oidc-agent configuration: %w", err)
		}

		transport = &tokenRoundTripper{
			token:     token,
			transport: transport,
		}
	} else if cluster.OIDCRefreshToken != "" {
		accessToken, err := cluster.getAccessToken()
		if err != nil {
			return nil, fmt.Errorf("unable to get the OIDC token from refresh token, please check your configuration: %w", err)
		}
		transport = &tokenRoundTripper{
			token:     accessToken,
			transport: transport,
		}
	} else {
		// Use basic auth
		transport = &basicAuthRoundTripper{
			username:  cluster.AuthUser,
			password:  cluster.AuthPassword,
			transport: transport,
		}
	}

	if len(args) != 0 {
		timeout = args[0]
	}

	return &http.Client{
		Transport: transport,
		Timeout:   time.Second * time.Duration(timeout),
	}, nil
}

// GetClient returns an HTTP client to communicate with the cluster
func (cluster *Cluster) GetClient(args ...int) *http.Client {
	client, err := cluster.GetClientSafe(args...)
	if err != nil {
		fmt.Printf("Unable to create the HTTP client: %v\n", err)
		os.Exit(1)
	}
	return client
}

// GetClusterInfo returns info from an OSCAR cluster
func (cluster *Cluster) GetClusterInfo() (info types.Info, err error) {
	getInfoURL, err := url.Parse(cluster.Endpoint)
	if err != nil {
		return info, ErrParsingEndpoint
	}
	getInfoURL.Path = path.Join(getInfoURL.Path, infoPath)

	req, err := http.NewRequest(http.MethodGet, getInfoURL.String(), nil)
	if err != nil {
		return info, ErrMakingRequest
	}

	client, err := cluster.GetClientSafe()
	if err != nil {
		return info, err
	}

	res, err := client.Do(req)
	if err != nil {
		return info, ErrSendingRequest
	}
	defer res.Body.Close()

	if err := CheckStatusCode(res); err != nil {
		return info, err
	}

	// Decode the response body into the info struct
	json.NewDecoder(res.Body).Decode(&info)

	return info, nil
}

// GetClusterConfig returns the config of an OSCAR cluster
func (cluster *Cluster) GetClusterConfig() (cfg types.Config, err error) {
	getConfigURL, err := url.Parse(cluster.Endpoint)
	if err != nil {
		return cfg, ErrParsingEndpoint
	}
	getConfigURL.Path = path.Join(getConfigURL.Path, configPath)

	req, err := http.NewRequest(http.MethodGet, getConfigURL.String(), nil)
	if err != nil {
		return cfg, ErrMakingRequest
	}

	client, err := cluster.GetClientSafe()
	if err != nil {
		return cfg, err
	}

	res, err := client.Do(req)
	if err != nil {
		return cfg, ErrSendingRequest
	}
	defer res.Body.Close()

	if err := CheckStatusCode(res); err != nil {
		return cfg, err
	}

	// Decode the response body into the info struct
	json.NewDecoder(res.Body).Decode(&cfg)

	return cfg, nil
}

// GetClusterStatus returns the status of an OSCAR cluster
func (cluster *Cluster) GetClusterStatus() (status StatusInfo, err error) {
	getStatusURL, err := url.Parse(cluster.Endpoint)
	if err != nil {
		return status, ErrParsingEndpoint
	}
	getStatusURL.Path = path.Join(getStatusURL.Path, statusPath)

	req, err := http.NewRequest(http.MethodGet, getStatusURL.String(), nil)
	if err != nil {
		return status, ErrMakingRequest
	}

	client, err := cluster.GetClientSafe()
	if err != nil {
		return status, err
	}

	res, err := client.Do(req)
	if err != nil {
		return status, ErrSendingRequest
	}
	defer res.Body.Close()

	if err := CheckStatusCode(res); err != nil {
		return status, err
	}

	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		return status, fmt.Errorf("unable to parse cluster status response: %w", err)
	}

	return status, nil
}

// CheckStatusCode checks if a cluster response is valid and returns an appropriate error if not
func CheckStatusCode(res *http.Response) error {
	if res.StatusCode >= 200 && res.StatusCode <= 204 {
		return nil
	}
	if res.StatusCode == 401 {
		return errors.New("invalid credentials")
	}
	if res.StatusCode == 404 {
		return errors.New("not found")
	}
	if res.StatusCode == 502 {
		return errors.New("the service is not ready yet, please wait until it's ready or check if something failed")
	}
	// Create an error from the failed response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("cannot read the response: %v", err)
	}
	return errors.New(string(body))
}

func getStringClaim(claims jwt.MapClaims, name string) (string, error) {
	value, ok := claims[name]
	if !ok {
		return "", fmt.Errorf("missing %q claim", name)
	}

	text, ok := value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("invalid %q claim", name)
	}

	return text, nil
}

func (cluster *Cluster) getAccessToken() (string, error) {
	claims := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(cluster.OIDCRefreshToken, claims)
	if err != nil {
		return "", fmt.Errorf("invalid OIDC refresh token: %w", err)
	}

	issuer, err := claims.GetIssuer()
	if err != nil {
		return "", fmt.Errorf("invalid OIDC refresh token issuer: %w", err)
	}

	scope, err := getStringClaim(claims, "scope")
	if err != nil {
		return "", fmt.Errorf("invalid OIDC refresh token: %w", err)
	}

	clientID, err := getStringClaim(claims, "azp")
	if err != nil {
		return "", fmt.Errorf("invalid OIDC refresh token: %w", err)
	}

	tokenURL, err := url.Parse(issuer)
	if err != nil {
		return "", fmt.Errorf("invalid issuer URL in OIDC refresh token: %w", err)
	}
	tokenURL.Path = path.Join(tokenURL.Path, "protocol/openid-connect/token")

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cluster.OIDCRefreshToken)
	form.Set("client_id", clientID)
	form.Set("scope", scope)

	req, err := http.NewRequest(http.MethodPost, tokenURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating OIDC token request: %w", err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Defensive timeout to avoid hanging the TUI when the IdP is slow/unreachable.
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending OIDC token request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 4096))
		if readErr != nil {
			return "", fmt.Errorf("OIDC token endpoint returned %s and the error body could not be read: %w", res.Status, readErr)
		}
		message := strings.TrimSpace(string(body))
		if message == "" {
			return "", fmt.Errorf("OIDC token endpoint returned %s", res.Status)
		}
		return "", fmt.Errorf("OIDC token endpoint returned %s: %s", res.Status, message)
	}

	var rrt ResponseRefreshToken
	if err := json.NewDecoder(res.Body).Decode(&rrt); err != nil {
		return "", fmt.Errorf("cannot decode OIDC token response: %w", err)
	}
	if rrt.AccessToken == "" {
		return "", errors.New("OIDC token response did not include an access_token")
	}

	return rrt.AccessToken, nil
}
