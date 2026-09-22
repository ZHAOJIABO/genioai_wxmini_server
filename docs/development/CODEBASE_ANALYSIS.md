# Codebase Analysis Report

This report analyzes the provided codebase to identify its primary programming language, technology stack, database architecture, and coding conventions.

## 1. Primary Programming Language

The project is primarily written in **Go (Golang)**, version **1.23**, as indicated by the `go.mod` file and the `.go` source files.

## 2. Technology Stack

The project utilizes the following key technologies:

*   **API Frameworks:**
    *   **gRPC:** (`google.golang.org/grpc v1.69.2`) Used for defining and serving RPC APIs.
    *   **gRPC-Gateway:** (`github.com/grpc-ecosystem/grpc-gateway/v2 v2.25.1`) Used to expose gRPC services as RESTful JSON APIs. A custom marshaler (`internal/common/grpc_gateway_marshal.go`) is implemented, possibly for handling request/response encryption.
*   **Database Clients & ORM:**
    *   **GORM:** (`gorm.io/gorm v1.25.12`) Used as the ORM for interacting with the MySQL database. MySQL driver: `gorm.io/driver/mysql v1.5.7`.
    *   **MongoDB Driver:** (`go.mongodb.org/mongo-driver v1.7.5`) Used for interacting with the MongoDB database.
    *   **Redis Client:** (`github.com/go-redis/redis v6.14.2+incompatible`) Used for interacting with Redis.
*   **Configuration:**
    *   **Viper:** (`github.com/spf13/viper v1.17.0`) Used for managing application configuration (implied by `go.mod` and `conf.GlobalConfig` usage).
*   **Logging:**
    *   **Zap:** (`go.uber.org/zap v1.21.0`) Used for structured, high-performance logging.
    *   **Lumberjack:** (`github.com/natefinch/lumberjack v2.0.0+incompatible`) Used for log file rotation (`internal/zlog/logger.go`).
*   **Websockets:**
    *   **Gorilla Websocket:** (`github.com/gorilla/websocket v1.5.3`) Used for real-time communication, likely for streaming responses (e.g., TTS audio in `internal/api/tts.go`, chat messages in `internal/api/chat.go`).
*   **Authentication:**
    *   **JWT:** (`github.com/golang-jwt/jwt/v5 v5.2.1`) Used for handling JSON Web Tokens, likely for access/refresh tokens (`internal/model/user.go`, `internal/api/user.go`).
*   **External Services/SDKs:**
    *   **Azure OpenAI:** (`github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai v0.6.2`) Integration with Azure's OpenAI services.
    *   **Aliyun OSS:** (`github.com/aliyun/aliyun-oss-go-sdk v3.0.2+incompatible`) Used for object storage (file uploads).
    *   **Tencent Cloud SMS:** (`github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms v1.0.959`) Used for sending SMS messages (likely verification codes).
    *   **Phonenumbers:** (`github.com/nyaruka/phonenumbers v1.5.0`) Library for parsing, formatting, and validating phone numbers.
*   **Database Migrations:**
    *   **golang-migrate:** (`github.com/golang-migrate/migrate/v4 v4.18.1`) Used for managing MySQL database schema migrations (`internal/db/migration.go`).
*   **Monitoring:**
    *   **Prometheus Client:** (`github.com/prometheus/client_golang v1.17.0`) Integrated for exposing application metrics.
*   **Build & Dependency Management:**
    *   **Go Modules:** Used for dependency management (`go.mod`, `go.sum`).
    *   **Mage:** (`github.com/magefile/mage v1.15.0`) Potentially used as a build tool (dependency present in `go.mod`).

## 3. Database Architecture & Usage

The project employs a multi-database strategy:

*   **MySQL:**
    *   **Client:** `gorm.io/gorm v1.25.12` with `gorm.io/driver/mysql v1.5.7`.
    *   **Usage:** Serves as the primary relational database for storing structured data. This includes user records (`UserRecord`, `UserPersonalInfo`, `UserDeviceInfo`), configuration (`Config`, `Model`, `Prompt`, `Workflow`), task metadata (`PictureTask`, `Voice`), chat metadata (`Chat`), upload records (`UploadFile`, `UploadFileInfo`), subscription/order data (`Order`, `Product`, `UserSubscription`), event tracking (`UserEvent`, `TrackingEvent`, `AttributionEvent`, `AsaEvent`), and other relational entities.
    *   **Conventions:** Uses a table prefix `va_` (`internal/db/mysql.go`). Auto-migrations are handled by GORM (`internal/db/mysql.go`), and schema migrations are managed by `golang-migrate` (`internal/db/migration.go`). Connection pooling is configured (`internal/db/mysql.go`).
*   **MongoDB:**
    *   **Client:** `go.mongodb.org/mongo-driver v1.7.5`.
    *   **Usage:** Primarily used for storing less structured or high-volume data like chat message history (`internal/model/message.go`). Collections appear to be dynamically named based on date (`internal/db/mongo.go`). It also stores detailed constellation analysis results (`ConstellationInfo` in `internal/model/constellation.go`).
    *   **Conventions:** Connection pooling is managed by the driver.
*   **Redis:**
    *   **Client:** `github.com/go-redis/redis v6.14.2+incompatible`.
    *   **Usage:** Utilized for various purposes:
        *   **Caching:** General caching (implied).
        *   **Task Queues:** Manages background tasks, specifically for constellation analysis (`internal/task/constellation_queue.go`).
        *   **Distributed Locks:** Ensures tasks like user amount resets and constellation analysis are not run concurrently by multiple instances (`internal/task/user_chat_amount.go`, `internal/task/constellation_queue.go`).
        *   **Rate Limiting/Counters:** Tracks user daily chat amounts (`internal/task/user_chat_amount.go`, `internal/service/subscribe.go`).
        *   **Temporary Data Storage:** Stores daily encouragement messages (`internal/task/daily_encouragement.go`).
    *   **Conventions:** Initialized with a connection pool (`internal/db/redis.go`). Uses specific key prefixes (e.g., `visionai:`).

## 4. Coding Style & Conventions

*   **Project Structure:** Follows a standard Go project layout with a clear separation of concerns within the `internal/` directory:
    *   `api/`: Handles gRPC service implementations and request/response logic.
    *   `service/`: Contains the core business logic.
    *   `dao/`: Data Access Objects for interacting with databases.
    *   `model/`: Defines data structures (database models and potentially DTOs).
    *   `db/`: Database initialization and connection management.
    *   `constants/`: Defines application-wide constants.
    *   `zlog/`: Custom logging setup.
    *   `common/`: Shared utilities, middleware helpers, context functions.
    *   `task/`: Background task processing logic.
    *   `bootstrap/`: Application initialization and dependency setup.
    *   `va_interface/`: Protocol Buffer definitions and generated code.
*   **Logging:**
    *   Uses `zap` for structured logging (`internal/zlog/logger.go`).
    *   Employs `zlog.LogWithContext(ctx)` to automatically enrich logs with contextual information like `trace_id` and `user_id` extracted from the `context.Context` (`internal/zlog/logger.go`).
    *   Log rotation is handled via `lumberjack`.
*   **Error Handling:**
    *   Uses standard Go error handling (`if err != nil`).
    *   Errors are sometimes wrapped with context using `fmt.Errorf` or `github.com/pkg/errors`.
    *   Defines specific error variables (e.g., `ErrInvalidUser`, `ErrInvalidTaskID` in `internal/api/picture_forge.go`).
    *   Standardized API error responses are generated using `BuildErrorResponse` helper, which includes logging the error and setting appropriate status codes/messages in the response header (`internal/api/common.go`).
*   **API Design:**
    *   Primarily gRPC-based, defined using Protocol Buffers (`internal/va_interface/`).
    *   Uses gRPC-Gateway to provide RESTful endpoints.
    *   Supports streaming responses for chat and TTS (`internal/api/chat.go`, `internal/api/tts.go`).
    *   Request/response headers (`RequestHeader`, `ResponseHeader`) are consistently used for metadata like request IDs, access tokens, and status codes (`internal/va_interface/common.pb.go`).
*   **Context Management:**
    *   `context.Context` is passed through call chains for cancellation, deadlines, and value propagation (e.g., user ID, trace ID, project ID).
    *   Helper functions exist for getting/setting values in context (`internal/common/context.go`, `internal/utils/context.go`).
*   **Dependency Injection:**
    *   A manual dependency injection approach is used via the `internal/bootstrap` package, which initializes DAOs, services, and other components and wires them together (`internal/bootstrap/service_provider.go`).
*   **Concurrency:**
    *   Goroutines are used for background tasks (`internal/task/`).
    *   Channels are used for communication between goroutines (e.g., task queues).
    *   `sync.Mutex` and `sync.RWMutex` are used for protecting shared resources.
    *   `errgroup` is used for managing groups of goroutines (`internal/task/user_chat_amount.go`).
*   **Testing:**
    *   Uses `testify` for assertions (`go.mod`).
    *   Includes helpers for setting up test dependencies (`internal/bootstrap/test_helper.go`, `internal/bootstrap/test_service_provider.go`).
```
