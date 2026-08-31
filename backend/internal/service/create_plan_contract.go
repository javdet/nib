package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/llm"
)

const CreatePlanContractToolName = "create_plan_contract"

var createPlanContractParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "contract": {
      "type": "object",
      "properties": {
        "shared": {
          "type": "array",
          "description": "Every name two or more stages must agree on.",
          "items": {
            "type": "object",
            "properties": {
              "key": { "type": "string", "description": "Stable identifier for this decision, e.g. cassandra.jmx.user" },
              "value": { "type": "string", "description": "The exact name, path or version every stage must use verbatim. Never a live secret." },
              "kind": { "type": "string", "description": "identifier | secret_path | endpoint | namespace | version | other" },
              "decidedBy": { "type": "string", "description": "DAG stage that creates or owns this thing" }
            },
            "required": ["key", "value"]
          }
        },
        "stages": {
          "type": "object",
          "description": "Keyed by DAG stage name.",
          "additionalProperties": {
            "type": "object",
            "properties": {
              "provides": { "type": "array", "items": { "type": "string" } },
              "requires": { "type": "array", "items": { "type": "string" } },
              "blocking": {
                "type": "boolean",
                "description": "True only when this stage cannot be planned until the producing stage has been researched. Rare: a value listed in shared already answers the dependency."
              }
            }
          }
        }
      },
      "required": ["shared", "stages"]
    }
  },
  "required": ["contract"]
}`)

// CreatePlanContractToolDef returns the LLM tool definition for persisting the
// cross-stage contract the stage planners share.
func CreatePlanContractToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: CreatePlanContractToolName,
		Description: "Save the cross-stage contract for this plan: every identifier, secret path, endpoint, namespace and version that more than one stage has to agree on. " +
			"Stages are planned in parallel by separate agents that cannot see each other's work, so a name left undecided here is a name each stage invents for itself. " +
			"Keys marked blocking are the rare case where a stage genuinely cannot be planned until an earlier one has been researched.",
		Parameters: createPlanContractParameters,
	}
}

// createPlanContractHandler returns a local tool handler bound to a specific dialog.
func (s *ChatService) createPlanContractHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw, ok := args["contract"]
		if !ok || raw == nil {
			return "contract is required", nil
		}

		data, err := json.Marshal(raw)
		if err != nil {
			return "contract must be a valid JSON object", nil
		}
		var contract PlanContract
		if err := json.Unmarshal(data, &contract); err != nil {
			return "contract does not match the expected shape: " + err.Error(), nil
		}

		// The DAG names the stages a contract may talk about. An unknown stage
		// is reported rather than rejected, so one stray name does not cost the
		// caller the whole contract.
		dag, found, err := s.ReadDAG(dialogID)
		if err != nil {
			return "", fmt.Errorf("read dag: %w", err)
		}
		var unknown []string
		if found {
			titles := dagStageTitles(dag)
			for name := range contract.Stages {
				if _, ok := matchDAGStage(titles, name); !ok {
					unknown = append(unknown, name)
				}
			}
			sort.Strings(unknown)
		}

		if err := os.MkdirAll(s.planContractsDir, 0o755); err != nil {
			return "", fmt.Errorf("create plan_contracts directory: %w", err)
		}
		dest := filepath.Join(s.planContractsDir, dialogID.String()+".json")
		if err := atomicfile.Write(dest, data); err != nil {
			return "", err
		}

		relPath := filepath.Join("plan_contracts", dialogID.String()+".json")
		out := fmt.Sprintf("Plan contract saved to %s with %d shared values across %d stages",
			relPath, len(contract.Shared), len(contract.Stages))
		if len(unknown) > 0 {
			out += fmt.Sprintf(". These stage names are not in the DAG and will be ignored: %s", strings.Join(unknown, ", "))
		}
		return out, nil
	}
}
