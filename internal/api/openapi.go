package api

const openAPISpec = `openapi: 3.1.0
info:
  title: Kanvas API
  version: 0.1.0
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
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      description: Personal Kanvas API key (knv_...). Browser clients may use the secure session cookie and X-CSRF-Token.
`
