package executors

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/akerl/speculate/creds"

	"github.com/akerl/timber/log"
	"github.com/aws/aws-sdk-go/service/sts"
)

var logger = log.NewLogger("speculate")

const (
	mfaCodeRegexString   = `^\d{6}$`
	mfaArnRegexString    = `^arn:aws(?:-us-gov)?:iam::\d+:mfa/[\w+=,.@-]+$`
	iamEntityRegexString = `^[\w+=,.@-]+$`
	accountIDRegexString = `^\d{12}$`
)

var mfaCodeRegex = regexp.MustCompile(mfaCodeRegexString)
var mfaArnRegex = regexp.MustCompile(mfaArnRegexString)
var iamEntityRegex = regexp.MustCompile(iamEntityRegexString)
var accountIDRegex = regexp.MustCompile(accountIDRegexString)

// Executor defines the interface for requesting a new set of AWS creds
type Executor interface {
	Execute() (creds.Creds, error)
	ExecuteWithCreds(creds.Creds) (creds.Creds, error)
	SetAccountID(string) error
	SetRoleName(string) error
	SetSessionName(string) error
	SetPolicy(string) error
	SetLifetime(int64) error
	SetMfa(bool) error
	SetMfaSerial(string) error
	SetMfaCode(string) error
	SetMfaPrompt(MfaPrompt) error
	GetAccountID() (string, error)
	GetRoleName() (string, error)
	GetSessionName() (string, error)
	GetPolicy() (string, error)
	GetLifetime() (int64, error)
	GetMfa() (bool, error)
	GetMfaSerial() (string, error)
	GetMfaCode() (string, error)
	GetMfaPrompt() (MfaPrompt, error)
}

// Lifetime object encapsulates the setup of session duration
type Lifetime struct {
	lifetimeInt int64
}

// SetLifetime allows setting the credential lifespan
func (l *Lifetime) SetLifetime(val int64) error {
	if val != 0 && (val < 900 || val > 3600) {
		return fmt.Errorf("lifetime must be between 900 and 3600: %d", val)
	}
	logger.InfoMsg(fmt.Sprintf("Setting lifetime to %d", val))
	l.lifetimeInt = val
	return nil
}

// GetLifetime returns the lifetime of the executor
func (l *Lifetime) GetLifetime() (int64, error) {
	if l.lifetimeInt == 0 {
		l.lifetimeInt = 3600
	}
	return l.lifetimeInt, nil
}

// Mfa object encapsulates the setup of MFA for API calls
type Mfa struct {
	useMfa    bool
	mfaSerial string
	mfaCode   string
	mfaPrompt MfaPrompt
}

// MfaPrompt interface describes an object which can prompt the user for their MFA
type MfaPrompt interface {
	Prompt() (string, error)
}

// SetMfa sets whether MFA is used
func (m *Mfa) SetMfa(val bool) error {
	logger.InfoMsg(fmt.Sprintf("Setting MFA: %t", val))
	m.useMfa = val
	return nil
}

// SetMfaSerial sets the ARN of the MFA device
func (m *Mfa) SetMfaSerial(val string) error {
	if val == "" || mfaArnRegex.MatchString(val) {
		logger.InfoMsg(fmt.Sprintf("Setting MFA serial: %s", val))
		m.mfaSerial = val
		return nil
	}
	return fmt.Errorf("MFA Serial is malformed: %s", val)
}

// SetMfaCode sets the OTP for MFA
func (m *Mfa) SetMfaCode(val string) error {
	if val == "" || mfaCodeRegex.MatchString(val) {
		logger.InfoMsg(fmt.Sprintf("Setting MFA code: %s", val))
		m.mfaCode = val
		return nil
	}
	return fmt.Errorf("MFA Code is malformed: %s", val)
}

// SetMfaPrompt provides a custom method for loading the MFA code
func (m *Mfa) SetMfaPrompt(val MfaPrompt) error {
	logger.InfoMsg("Setting MFA prompt function")
	m.mfaPrompt = val
	return nil
}

// GetMfa returns if MFA will be used
func (m *Mfa) GetMfa() (bool, error) {
	if !m.useMfa && m.mfaCode != "" {
		m.useMfa = true
	}
	return m.useMfa, nil
}

// GetMfaSerial returns the ARN of the MFA device
func (m *Mfa) GetMfaSerial() (string, error) {
	if m.mfaSerial == "" {
		c := creds.Creds{}
		var err error
		m.mfaSerial, err = c.MfaArn()
		if err != nil {
			return "", err
		}
		logger.InfoMsg(fmt.Sprintf("Using default value for MFA serial: %s", m.mfaSerial))
	}
	return m.mfaSerial, nil
}

// GetMfaCode returns the OTP to use
func (m *Mfa) GetMfaCode() (string, error) {
	if m.mfaCode == "" {
		mfaPrompt, err := m.GetMfaPrompt()
		if err != nil {
			return "", err
		}
		logger.InfoMsg("Calling MFA Prompt function")
		mfa, err := mfaPrompt.Prompt()
		if err != nil {
			return "", err
		}
		m.mfaCode = mfa
	}
	return m.mfaCode, nil
}

// GetMfaPrompt returns the function to use for asking the user for an MFA code
func (m *Mfa) GetMfaPrompt() (MfaPrompt, error) {
	if m.mfaPrompt == nil {
		logger.InfoMsg("Using default value for MFA prompt function")
		m.mfaPrompt = &DefaultMfaPrompt{}
	}
	return m.mfaPrompt, nil
}

// DefaultMfaPrompt defines the standard CLI-based MFA prompt
type DefaultMfaPrompt struct{}

// Prompt asks the user for their MFA token
func (p *DefaultMfaPrompt) Prompt() (string, error) {
	mfaReader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stderr, "MFA Code: ")
	mfa, err := mfaReader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(mfa), nil
}

func (m *Mfa) configureMfa(paramsIface interface{}) error {
	useMfa, err := m.GetMfa()
	if err != nil {
		return err
	}
	if !useMfa {
		return nil
	}

	mfaCode, err := m.GetMfaCode()
	if err != nil {
		return err
	}
	mfaSerial, err := m.GetMfaSerial()
	if err != nil {
		return err
	}

	switch params := paramsIface.(type) {
	case *sts.AssumeRoleInput:
		params.TokenCode = &mfaCode
		params.SerialNumber = &mfaSerial
	case *sts.GetSessionTokenInput:
		params.TokenCode = &mfaCode
		params.SerialNumber = &mfaSerial
	default:
		return fmt.Errorf("expected AssumeRoleInput or GetSessionTokenInput, received %T", params)
	}
	return nil
}
