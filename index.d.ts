// Type definitions for the bmcptools metadata module.
//
// This package ships static metadata only; the MCP server itself is a
// standalone Go binary (see the README "Download" section).

/** npm package / server name. */
export declare const name: string;

/** Server version, kept in sync with the package version. */
export declare const version: string;

/** One-line server description. */
export declare const description: string;

/** Tool-group name as accepted by the server's `--disable` flag. */
export type ToolGroup =
  | 'file'
  | 'multi'
  | 'dir'
  | 'search'
  | 'exec'
  | 'system'
  | 'user';

/** Map of each tool group to the tool names it registers. */
export declare const groups: Record<ToolGroup, string[]>;

/** Flat list of every tool name exposed by the server. */
export declare const tools: string[];
