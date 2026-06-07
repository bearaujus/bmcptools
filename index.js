'use strict';

// Metadata module for bmcptools.
//
// The actual server is a standalone Go binary (see the README "Download"
// section). This npm package ships no runtime and performs no I/O, no shell
// execution, and no network access. It exposes static metadata describing the
// MCP server's tool surface so JS/TS tooling can reference tool and group
// names without parsing the README.

/** npm package / server name. */
const name = 'bmcptools';

/** Server version, kept in sync with the package version. */
const version = '1.10.1';

/** One-line server description. */
const description =
  'MCP server exposing 44 developer tools to any MCP-compatible LLM client.';

/**
 * Tool groups keyed by the name accepted by the server's `--disable` flag
 * (and the `BMCPTOOLS_DISABLE` env var), each mapped to the tools it registers.
 */
const groups = {
  file: [
    'read_file',
    'write_file',
    'append_to_file',
    'edit_file',
    'delete_file',
    'copy_file',
    'move_file',
    'get_file_info',
    'path_exists',
    'diff_files',
    'calculate_checksum',
    'create_symlink',
    'compress_files',
    'extract_archive',
  ],
  multi: [
    'read_multiple_files',
    'write_multiple_files',
    'find_replace_in_files',
    'path_exists_batch',
    'get_multiple_file_info',
    'delete_files',
    'copy_paths',
    'move_paths',
  ],
  dir: ['list_directory', 'directory_tree', 'create_directory', 'delete_directory'],
  search: ['search_files', 'grep_files'],
  exec: ['get_working_directory', 'run_command', 'open_in_app', 'get_env'],
  system: [
    'http_request',
    'download_file',
    'clipboard_read',
    'clipboard_write',
    'list_processes',
    'get_system_info',
  ],
  user: [
    'ask_user',
    'get_user_response',
    'update_dialog',
    'cancel_ask_user',
    'notify_user',
    'rest',
  ],
};

/** Flat list of every tool name exposed by the server. */
const tools = Object.keys(groups).reduce(
  (all, group) => all.concat(groups[group]),
  []
);

module.exports = { name, version, description, groups, tools };
