package api

const openAPISpec = `openapi: 3.1.0
info:
  title: Kanvas API
  version: 0.2.0
  description: ACL-aware Wiki, administration, migration, and personal key API.
servers:
  - url: /api/v1
security:
  - bearerAuth: []
paths:
  /version:
    get:
      security: []
      operationId: getVersion
      responses: { "200": { description: Build information } }
  /auth/login:
    post:
      security: []
      operationId: localLogin
      responses: { "200": { description: Authenticated } }
  /spaces:
    get:
      operationId: listSpaces
      responses: { "200": { description: ACL-filtered spaces } }
    post:
      operationId: createSpace
      responses: { "201": { description: Space created } }
  /spaces/{spaceId}/pages:
    get:
      operationId: listSpacePages
      parameters: [{ name: spaceId, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { "200": { description: ACL-filtered pages } }
    post:
      operationId: createPage
      parameters: [{ name: spaceId, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { "201": { description: Page created } }
  /pages/{pageId}:
    get:
      operationId: getPage
      parameters: [{ name: pageId, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { "200": { description: Page with current immutable version } }
    put:
      operationId: updatePage
      parameters: [{ name: pageId, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { "200": { description: Updated page }, "409": { description: Version conflict } }
  /search:
    get:
      operationId: searchPages
      parameters: [{ name: q, in: query, schema: { type: string } }]
      responses: { "200": { description: Permission-filtered search results } }
  /personal/api-keys:
    get:
      operationId: listPersonalAPIKeys
      responses: { "200": { description: Personal API keys } }
    post:
      operationId: createPersonalAPIKey
      responses: { "201": { description: API key; token is returned once } }
  /admin/migration:
    get:
      operationId: getMigrationDashboard
      responses: { "200": { description: Migration state, checks, and latest discovery } }
  /admin/migration/discovery:
    post:
      operationId: discoverConfluenceSchema
      responses: { "200": { description: Read-only legacy schema discovery result } }
  /admin/migration/snapshot:
    post:
      operationId: startInitialSnapshot
      requestBody:
        required: false
        content:
          application/json:
            schema: { $ref: '#/components/schemas/SnapshotOptions' }
      responses:
        "202": { description: Restartable snapshot job accepted }
        "409": { description: Another snapshot job is active }
        "412": { description: Schema discovery has not completed }
  /admin/migration/jobs:
    get:
      operationId: listMigrationJobs
      responses: { "200": { description: Migration jobs ordered by creation time } }
  /admin/migration/jobs/{jobId}:
    get:
      operationId: getMigrationJob
      parameters: [{ name: jobId, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { "200": { description: Job status, progress, options, and checkpoint } }
  /admin/migration/jobs/{jobId}/items:
    get:
      operationId: listMigrationItems
      parameters:
        - { name: jobId, in: path, required: true, schema: { type: string, format: uuid } }
        - { name: status, in: query, required: false, schema: { type: string, enum: [PENDING, RUNNING, COMPLETE, FAILED] } }
      responses: { "200": { description: Per-record migration status and retry details } }
  /admin/migration/jobs/{jobId}/cancel:
    post:
      operationId: cancelMigrationJob
      parameters: [{ name: jobId, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { "204": { description: Cancellation requested } }
  /admin/migration/jobs/{jobId}/resume:
    post:
      operationId: resumeMigrationJob
      parameters: [{ name: jobId, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { "202": { description: Job resumed; completed records are skipped } }
  /admin/migration/macros:
    get:
      operationId: getMacroCompatibility
      responses: { "200": { description: Native and unsupported macro conversion coverage } }
  /admin/migration/unsupported:
    get:
      operationId: listUnsupportedContent
      responses: { "200": { description: Unsupported macros, invalid XHTML, and orphan records } }
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      description: Personal Kanvas API key (knv_...). Browser clients may use the secure session cookie and X-CSRF-Token.
  schemas:
    SnapshotOptions:
      type: object
      properties:
        batchSize: { type: integer, minimum: 10, maximum: 5000, default: 500 }
        includeUsers: { type: boolean, default: true }
        includeGroups: { type: boolean, default: true }
        includeSpaces: { type: boolean, default: true }
        includePages: { type: boolean, default: true }
        includeComments: { type: boolean, default: true }
        includeAttachmentMetadata: { type: boolean, default: true }
        includePermissions: { type: boolean, default: true }
`
