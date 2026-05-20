project {
  name            = "dex"
  default_platform = "claude-code"

  git_exclude = true
}

settings "project_permissions" {
  claude {
    enable_all_project_mcp_servers = true
  }
}
