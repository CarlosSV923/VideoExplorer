// PreToolUse hook for the video-builder subagent: the spec, the task list, CLAUDE.md
// and the agent configuration belong to the orchestrator, so the subagent may read
// them (Read tool) but never write them. Exit code 2 blocks the tool call.
import { readFileSync } from 'node:fs'
import path from 'node:path'

const PROTECTED = ['docs/spec.md', 'docs/tasks.md', 'CLAUDE.md']
const PROTECTED_DIRS = ['.claude/']
const ALLOWED = ['.claude/launch.json']
const MESSAGE =
  'Blocked: docs/spec.md, docs/tasks.md, CLAUDE.md and .claude/ are managed by the orchestrator. ' +
  'Read them with the Read tool; report any change they need in your final report instead.'

function block() {
  process.stderr.write(MESSAGE)
  process.exit(2)
}

// Fail closed: an unreadable payload must not let a write through.
let input
try {
  input = JSON.parse(readFileSync(0, 'utf8'))
} catch {
  block()
}
const toolInput = input.tool_input ?? {}
const projectDir = process.env.CLAUDE_PROJECT_DIR ?? input.cwd ?? process.cwd()
const norm = (p) => p.replaceAll('\\', '/').toLowerCase()

function isProtectedPath(filePath) {
  const rel = norm(path.relative(projectDir, path.resolve(projectDir, filePath)))
  if (ALLOWED.some((a) => rel === norm(a))) return false
  return PROTECTED.some((p) => rel === norm(p)) || PROTECTED_DIRS.some((d) => rel.startsWith(norm(d)))
}

const filePath = toolInput.file_path ?? toolInput.notebook_path
if (typeof filePath === 'string' && isProtectedPath(filePath)) block()

// Shell commands: any mention of a protected file is rejected (reading goes through Read).
const command = toolInput.command
if (typeof command === 'string') {
  const c = norm(command)
  const mentionsClaudeDir = /\.claude\//.test(c.replaceAll('.claude/launch.json', ''))
  if (/(^|[^a-z0-9_-])(spec\.md|tasks\.md|claude\.md)\b/.test(c) || mentionsClaudeDir) block()
}

process.exit(0)
