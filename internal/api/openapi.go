package api

const openAPISpec = `openapi: 3.1.0
info:
  title: Kanvas API
  version: 0.4.0
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
  /admin/overview:
    get:
      operationId: getAdminOverview
      responses: { "200": { description: Service-wide user, content, session, key, audit, and exception counters } }
  /admin/users:
    get:
      operationId: listAdminUsers
      parameters: [{ name: q, in: query, required: false, schema: { type: string } }]
      responses: { "200": { description: Users with group counts and account state } }
  /admin/users/{userId}:
    patch:
      operationId: updateAdminUser
      parameters: [{ name: userId, in: path, required: true, schema: { type: string, format: uuid } }]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/AdminUserUpdate' }
      responses:
        "200": { description: User role and state updated; disabling revokes sessions and API keys }
        "409": { description: Self-disable or last-active-administrator guard rejected the update }
  /admin/groups:
    get:
      operationId: listAdminGroups
      responses: { "200": { description: Groups and member counts } }
    post:
      operationId: createAdminGroup
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/AdminGroupCreate' }
      responses:
        "201": { description: Group created }
        "409": { description: Group name already exists }
  /admin/groups/{groupId}/members:
    get:
      operationId: listAdminGroupMembers
      parameters: [{ name: groupId, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { "200": { description: Group members } }
    post:
      operationId: addAdminGroupMember
      parameters: [{ name: groupId, in: path, required: true, schema: { type: string, format: uuid } }]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [userId]
              properties: { userId: { type: string, format: uuid } }
      responses: { "200": { description: Membership added idempotently } }
  /admin/groups/{groupId}/members/{userId}:
    delete:
      operationId: removeAdminGroupMember
      parameters:
        - { name: groupId, in: path, required: true, schema: { type: string, format: uuid } }
        - { name: userId, in: path, required: true, schema: { type: string, format: uuid } }
      responses: { "204": { description: Membership removed }, "404": { description: Membership not found } }
  /admin/spaces:
    get:
      operationId: listAdminSpaces
      responses: { "200": { description: Active and archived spaces with page and attachment counts } }
  /admin/spaces/{spaceId}:
    patch:
      operationId: updateAdminSpaceStatus
      parameters: [{ name: spaceId, in: path, required: true, schema: { type: string, format: uuid } }]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [status]
              properties: { status: { type: string, enum: [ACTIVE, ARCHIVED] } }
      responses: { "200": { description: Space archived or restored } }
  /admin/settings:
    get:
      operationId: listAdminSettings
      responses: { "200": { description: Managed settings and redacted environment connection fingerprints } }
    put:
      operationId: putAdminSetting
      responses: { "200": { description: Validated setting encrypted when marked secret and saved with audit history } }
  /admin/audit:
    get:
      operationId: listAuditEvents
      parameters:
        - { name: q, in: query, required: false, schema: { type: string } }
        - { name: action, in: query, required: false, schema: { type: string } }
      responses: { "200": { description: Filtered administration audit events } }
  /admin/status:
    get:
      operationId: getAdminStatus
      responses: { "200": { description: Database pool, Go runtime, memory, uptime, and build status } }
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
  /admin/migration/reconciliation:
    post:
      operationId: startMigrationReconciliation
      responses:
        "202": { description: Independent reconciliation job accepted }
        "409": { description: Another reconciliation job is active }
        "412": { description: A completed Snapshot is required }
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
      parameters:
        - { name: jobId, in: query, required: false, schema: { type: string, format: uuid } }
        - { name: status, in: query, required: false, schema: { type: string, enum: [OPEN, APPROVED, RESOLVED] } }
        - { name: kind, in: query, required: false, schema: { type: string } }
        - { name: q, in: query, required: false, schema: { type: string } }
        - { name: limit, in: query, required: false, schema: { type: integer, minimum: 1, maximum: 200, default: 100 } }
        - { name: offset, in: query, required: false, schema: { type: integer, minimum: 0, default: 0 } }
      responses:
        "200":
          description: Filtered unsupported content from the latest Snapshot by default
          content:
            application/json:
              schema: { $ref: '#/components/schemas/UnsupportedContentPage' }
  /admin/migration/unsupported/{itemId}:
    patch:
      operationId: decideUnsupportedContent
      parameters: [{ name: itemId, in: path, required: true, schema: { type: string, format: uuid } }]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/UnsupportedContentDecision' }
      responses:
        "200": { description: Decision recorded and related readiness checks refreshed }
        "400": { description: Invalid status or missing rationale }
        "404": { description: Unsupported item not found }
  /admin/migration/unsupported/bulk:
    post:
      operationId: bulkDecideUnsupportedContent
      requestBody:
        required: true
        content:
          application/json:
            schema:
              allOf:
                - { $ref: '#/components/schemas/UnsupportedContentDecision' }
                - type: object
                  required: [ids]
                  properties:
                    ids: { type: array, minItems: 1, maxItems: 500, items: { type: string, format: uuid } }
      responses:
        "200": { description: Bulk decision recorded atomically and related readiness checks refreshed }
        "400": { description: Invalid status, item count, or missing rationale }
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      description: Personal Kanvas API key (knv_...). Browser clients may use the secure session cookie and X-CSRF-Token.
  schemas:
    AdminUserUpdate:
      type: object
      required: [role, status]
      properties:
        role: { type: string, enum: [USER, ADMIN] }
        status: { type: string, enum: [ACTIVE, DISABLED] }
    AdminGroupCreate:
      type: object
      required: [name]
      properties:
        name: { type: string, minLength: 1, maxLength: 120 }
        description: { type: string }
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
    UnsupportedContentDecision:
      type: object
      required: [status]
      properties:
        status: { type: string, enum: [OPEN, APPROVED, RESOLVED] }
        resolution: { type: string, maxLength: 2000, description: Required for APPROVED and RESOLVED }
    UnsupportedContentItem:
      type: object
      required: [id, legacyId, kind, name, status, occurrenceCount, sample, resolution, createdAt, updatedAt]
      properties:
        id: { type: string, format: uuid }
        jobId: { type: string, format: uuid }
        pageId: { type: string, format: uuid }
        legacyId: { type: string }
        kind: { type: string }
        name: { type: string }
        status: { type: string, enum: [OPEN, APPROVED, RESOLVED] }
        occurrenceCount: { type: integer, format: int64 }
        sample: { type: string }
        resolution: { type: string }
        resolvedBy: { type: string, format: uuid }
        resolvedAt: { type: string, format: date-time }
        createdAt: { type: string, format: date-time }
        updatedAt: { type: string, format: date-time }
    UnsupportedContentPage:
      type: object
      required: [items, summary, filteredTotal, limit, offset]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/UnsupportedContentItem' } }
        summary:
          type: object
          required: [total, open, approved, resolved, byKind]
          properties:
            total: { type: integer, format: int64 }
            open: { type: integer, format: int64 }
            approved: { type: integer, format: int64 }
            resolved: { type: integer, format: int64 }
            byKind: { type: object, additionalProperties: { type: integer, format: int64 } }
        snapshotJobId: { type: string, format: uuid }
        filteredTotal: { type: integer, format: int64 }
        limit: { type: integer }
        offset: { type: integer }
`
