package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

const companyVariableScope = "global"

// ErrInvalidGitEmail is returned when company git email is non-empty but invalid.
var ErrInvalidGitEmail = errors.New("invalid git email address")

// CompanyInfo holds company metadata stored as prompt variables.
type CompanyInfo struct {
	CompanyName          string `json:"companyName"`
	CompanyDescription   string `json:"companyDescription"`
	VersionControlSystem string `json:"versionControlSystem"`
	GitBaseURL           string `json:"gitBaseUrl"`
	GitUsername          string `json:"gitUsername"`
	GitEmail             string `json:"gitEmail"`
	CICDSystem           string `json:"ciCdSystem"`
	TaskTracker          string `json:"taskTracker"`
	IssueProject         string `json:"issueProject"`
	Wiki                 string `json:"wiki"`
	Messenger            string `json:"messenger"`
}

type companyField struct {
	jsonKey      string
	varName      string
	description  string
	defaultValue string
	getValue     func(*CompanyInfo) string
	setValue     func(*CompanyInfo, string)
}

var companyFields = []companyField{
	{
		jsonKey:     "companyName",
		varName:     "CompanyName",
		description: "Company name",
		getValue:    func(c *CompanyInfo) string { return c.CompanyName },
		setValue:    func(c *CompanyInfo, v string) { c.CompanyName = v },
	},
	{
		jsonKey:     "companyDescription",
		varName:     "CompanyDescription",
		description: "Company description",
		getValue:    func(c *CompanyInfo) string { return c.CompanyDescription },
		setValue:    func(c *CompanyInfo, v string) { c.CompanyDescription = v },
	},
	{
		jsonKey:     "versionControlSystem",
		varName:     "VersionControlSystem",
		description: "Version control system",
		getValue:    func(c *CompanyInfo) string { return c.VersionControlSystem },
		setValue:    func(c *CompanyInfo, v string) { c.VersionControlSystem = v },
	},
	{
		jsonKey:     "gitBaseUrl",
		varName:     "GitBaseURL",
		description: "GIT base URL",
		getValue:    func(c *CompanyInfo) string { return c.GitBaseURL },
		setValue:    func(c *CompanyInfo, v string) { c.GitBaseURL = v },
	},
	{
		jsonKey:     "gitUsername",
		varName:     "GitUsername",
		description: "Git username",
		getValue:    func(c *CompanyInfo) string { return c.GitUsername },
		setValue:    func(c *CompanyInfo, v string) { c.GitUsername = v },
	},
	{
		jsonKey:     "gitEmail",
		varName:     "GitEmail",
		description: "Git email",
		getValue:    func(c *CompanyInfo) string { return c.GitEmail },
		setValue:    func(c *CompanyInfo, v string) { c.GitEmail = v },
	},
	{
		jsonKey:     "ciCdSystem",
		varName:     "CICDSystem",
		description: "CI/CD system",
		getValue:    func(c *CompanyInfo) string { return c.CICDSystem },
		setValue:    func(c *CompanyInfo, v string) { c.CICDSystem = v },
	},
	{
		jsonKey:     "taskTracker",
		varName:     "TaskTracker",
		description: "Task tracker",
		getValue:    func(c *CompanyInfo) string { return c.TaskTracker },
		setValue:    func(c *CompanyInfo, v string) { c.TaskTracker = v },
	},
	{
		jsonKey:      "issueProject",
		varName:      "IssueProject",
		description:  "Issue project",
		defaultValue: "DEVOPS",
		getValue:     func(c *CompanyInfo) string { return c.IssueProject },
		setValue:     func(c *CompanyInfo, v string) { c.IssueProject = v },
	},
	{
		jsonKey:     "wiki",
		varName:     "Wiki",
		description: "Wiki",
		getValue:    func(c *CompanyInfo) string { return c.Wiki },
		setValue:    func(c *CompanyInfo, v string) { c.Wiki = v },
	},
	{
		jsonKey:     "messenger",
		varName:     "Messenger",
		description: "Command messenger",
		getValue:    func(c *CompanyInfo) string { return c.Messenger },
		setValue:    func(c *CompanyInfo, v string) { c.Messenger = v },
	},
}

// CompanyService manages company metadata backed by prompt variables.
type CompanyService struct {
	repo repository.VariableRepository
}

func NewCompanyService(repo repository.VariableRepository) *CompanyService {
	return &CompanyService{repo: repo}
}

// EnsureDefaults seeds company variables when missing.
func (s *CompanyService) EnsureDefaults(ctx context.Context) error {
	for _, field := range companyFields {
		if err := s.repo.EnsureBuiltin(ctx, domain.PromptVariable{
			Scope:       companyVariableScope,
			Name:        field.varName,
			Description: field.description,
			Value:       field.defaultValue,
			Kind:        VariableKindString,
		}); err != nil {
			return fmt.Errorf("ensure company variable %s: %w", field.varName, err)
		}
	}
	return nil
}

// Get returns current company metadata from prompt variables.
func (s *CompanyService) Get(ctx context.Context) (CompanyInfo, error) {
	vars, err := s.repo.LoadAll(ctx, nil)
	if err != nil {
		return CompanyInfo{}, fmt.Errorf("load company variables: %w", err)
	}

	scopeVars := vars[companyVariableScope]
	info := CompanyInfo{}
	for _, field := range companyFields {
		value := ""
		if scopeVars != nil {
			value = asString(scopeVars[field.varName])
		}
		field.setValue(&info, value)
	}
	return info, nil
}

// Save persists company metadata as prompt variables.
func (s *CompanyService) Save(ctx context.Context, info CompanyInfo) (CompanyInfo, error) {
	info.GitUsername = strings.TrimSpace(info.GitUsername)
	info.GitEmail = strings.TrimSpace(info.GitEmail)
	if info.GitEmail != "" {
		if _, err := mail.ParseAddress(info.GitEmail); err != nil {
			return CompanyInfo{}, ErrInvalidGitEmail
		}
	}

	if info.CICDSystem == "" {
		info.CICDSystem = info.VersionControlSystem
	}

	for _, field := range companyFields {
		if _, err := s.repo.Upsert(ctx, domain.PromptVariable{
			Scope:       companyVariableScope,
			Name:        field.varName,
			Description: field.description,
			Value:       field.getValue(&info),
			Kind:        VariableKindString,
			Deletable:   false,
		}); err != nil {
			return CompanyInfo{}, fmt.Errorf("save company variable %s: %w", field.varName, err)
		}
	}
	return info, nil
}
