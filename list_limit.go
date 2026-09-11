package mwanachamagit

// maxListPage caps every List* method reachable through the /mcp surface
// (mwanachama-backend-api-shared's git_list_* tools) so a caller cannot
// force an unbounded row scan. See mwanachama-backend-api-kazi's board item
// K12.
const maxListPage = 500
