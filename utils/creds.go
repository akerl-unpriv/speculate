package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
)

type Creds struct {
	AccessKey, SecretKey, SessionToken string
}

func (c *Creds) New(argCreds map[string]string) error {
	required := []string{"AccessKey", "SecretKey", "SessionToken"}
	for _, key := range required {
		elem, ok := argCreds[key]
		if !ok || elem == "" {
			return fmt.Errorf("Missing required key for Creds: %s", key)
		}
	}
	c.AccessKey = argCreds["AccessKey"]
	c.SecretKey = argCreds["SecretKey"]
	c.SessionToken = argCreds["SessionToken"]
	return nil
}

func (c *Creds) NewFromEnv() error {
	envCreds := make(map[string]string)
	for k, v := range translations["envvar"] {
		if envCreds[v] == "" {
			envCreds[v] = os.Getenv(k)
		}
	}
	return c.New(envCreds)
}

var translations = map[string]map[string]string{
	"envvar": {
		"AWS_ACCESS_KEY_ID":     "AccessKey",
		"AWS_SECRET_ACCESS_KEY": "SecretKey",
		"AWS_SESSION_TOKEN":     "SessionToken",
		"AWS_SECURITY_TOKEN":    "SessionToken",
	},
	"console": {
		"sessionId":    "AccessKey",
		"sessionKey":   "SecretKey",
		"sessionToken": "SessionToken",
	},
}

func (c Creds) toMap() map[string]string {
	return map[string]string{
		"AccessKey":    c.AccessKey,
		"SecretKey":    c.SecretKey,
		"SessionToken": c.SessionToken,
	}
}

func (c Creds) translate(dictionary map[string]string) map[string]string {
	old := c.toMap()
	new := make(map[string]string)
	for k, v := range dictionary {
		new[k] = old[v]
	}
	return new
}

// ToEnvVars returns environment variables suitable for eval-ing into the shell
func (c Creds) ToEnvVars() []string {
	envCreds := c.translate(translations["envvar"])
	var res []string
	for k, v := range envCreds {
		res = append(res, fmt.Sprintf("export %s=%s", k, v))
	}
	sort.Strings(res)
	return res
}

var consoleTokenURL = "https://signin.aws.amazon.com/federation"

type consoleTokenResponse struct {
	SigninToken string
}

func (c Creds) toConsoleToken() (string, error) {
	args := []string{"Action=getSigninToken"}

	consoleCreds := c.translate(translations["console"])
	jsonCreds, err := json.Marshal(consoleCreds)
	if err != nil {
		return "", err
	}
	urlCreds := url.QueryEscape(string(jsonCreds))
	paramCreds := fmt.Sprintf("Session=%s", urlCreds)
	args = append(args, paramCreds)

	argString := strings.Join(args, "&")
	url := strings.Join([]string{consoleTokenURL, argString}, "?")

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	tokenObj := consoleTokenResponse{}
	if err := json.Unmarshal(body, &tokenObj); err != nil {
		return "", err
	}

	return tokenObj.SigninToken, nil
}

// ToConsoleURL returns a console URL for the role
func (c Creds) ToConsoleURL() (string, error) {
	consoleToken, err := c.toConsoleToken()
	if err != nil {
		return "", err
	}
	urlParts := []string{
		"https://signin.aws.amazon.com/federation",
		"?Action=login",
		"&Issuer=",
		"&Destination=",
		url.QueryEscape("https://console.aws.amazon.com/"),
		"&SigninToken=",
		consoleToken,
	}
	urlString := strings.Join(urlParts, "")
	return urlString, nil
}