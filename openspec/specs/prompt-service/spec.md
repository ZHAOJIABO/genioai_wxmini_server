# prompt-service Specification

## Purpose
TBD - created by archiving change refactor-prompt-service-directory. Update Purpose after archive.
## Requirements
### Requirement: Prompt Service Module Structure
The Prompt service code SHALL be organized in a dedicated `internal/service/prompt/` directory to improve code cohesion and maintainability.

#### Scenario: Module directory structure
- **WHEN** developers navigate to the prompt service code
- **THEN** all prompt-related service files are located in `internal/service/prompt/`
- **AND** the directory contains:
  - `prompt.go` - Core prompt service (PromptService)
  - `prompt_kind.go` - Prompt kind service (PromptKindService)
  - `optimizer.go` - Prompt optimizer service (OptimizerService)
  - `inspiration.go` - Inspiration prompt service (InspirationService)
  - `repository.go` - Repository interface definitions
  - Related test files

#### Scenario: Import path usage
- **WHEN** other modules need to use prompt services
- **THEN** they import `"va_visionai_server/internal/service/prompt"`
- **AND** access services via `prompt.PromptService`, `prompt.OptimizerService`, etc.

