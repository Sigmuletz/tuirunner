# tuirunner

A terminal UI script runner. It reads plain-text "library" files full of
named shell commands with `{{PLACEHOLDER}}` slots, lets you browse and
fill them in with arrow keys, and then hands the finished command
straight to your terminal to run — SSH sessions, `psql`, `oc rsh`, all
work exactly as if you'd typed them yourself.

## Requirements

- Go 1.24 or newer (only needed to build; the compiled binary has no
  runtime dependency on Go).
- A Linux/WSL terminal (uses `syscall.Exec` under the hood).

## Install / build

```
cd /home/sigma/claude/tuirunner
go build -o tuirunner .
```

This produces a single binary, `tuirunner`, in the project directory.

To run it from anywhere, put it on your `$PATH` — a symlink is fine, the
binary resolves its **real** path at startup:

```
ln -s /home/sigma/claude/tuirunner/tuirunner ~/.local/bin/tuirunner
```

**Important:** wherever the real binary lives, its profile data lives
right next to it (see below). Symlinking is fine; *moving* the compiled
binary without its containing directory is not — copy the whole project
directory if you relocate it.

## How it's laid out on disk

```
tuirunner/                  <- directory containing the binary itself
  tuirunner                  (the compiled binary)
  history.yaml                (global, not tied to any one profile)
  default/                    (a profile)
    libraries/
      connect_library
      curl_library
      ...
    favorites.yaml
    vars.yaml
  another-profile/            (you can have as many as you want)
    libraries/
    favorites.yaml
    vars.yaml
```

- **Profiles** are just directories sitting next to the binary, each
  with its own `libraries/` folder. tuirunner never creates a profile
  directory for you — if you ask for one that doesn't exist, it exits
  with an error. To add a profile, create the directory and its
  `libraries/` subfolder yourself.
- **`favorites.yaml`** and **`vars.yaml`** *inside* an existing profile
  *are* auto-created the first time that profile loads, if missing.
- **`history.yaml`** lives at the top level, shared by all profiles —
  it remembers your last profile, tab, script and every variable value,
  so relaunching with no arguments drops you back where you left off. It
  also keeps a log of your last 10 *executed* commands (profile,
  library, script and the variable values used), browsable with `Ctrl+H`
  in the UI.

### Library file format

Each file in `libraries/` is a plain-text list of commands, entries
separated by a blank line:

```
name|command
another_name|command with {{PLACEHOLDER}} substitution

multi_line_name|first part \
  continues here \
  and ends here
```

- Everything before the first `|` is the entry's display name.
- A trailing `` \ `` continues the command onto the next line (like a
  real shell line continuation) — this is preserved in the preview, but
  flattened into one line before it's actually run.
- `{{NAME}}` anywhere in a command becomes a fillable variable.
- The library's tab label in the UI is its filename with a trailing
  `_library` stripped (`connect_library` → `connect`), tabs shown in
  alphabetical order.

### `vars.yaml` — must-have and optional variables

```yaml
must_have:
  - VSS
  - KVNR_LIST

optional:
  - name: ENV
    default: prod
    choices: [prod, preprod, dev]
  - name: TENANT
    default: abc
  - name: DIRECTION
    choices: [ASC, DESC]
  - URL
```

- **`must_have`** variables always appear in the input form for that
  profile, regardless of which script is selected, and must be filled
  before anything in that profile can run. Their order here is also the
  order of positional CLI arguments (see below).
- **`optional`** variables also always appear in the form (after the
  must-haves), but are only required when the selected script actually
  uses them as a `{{PLACEHOLDER}}`.

Every entry, in either list, is either a bare name or a mapping with:

- **`default`** — the value the field starts with when there's no
  remembered value for it yet (a new profile, a profile switch, `Ctrl+N`,
  or a history run that predates the variable). Once you've edited the
  value, your last-used value is remembered in `history.yaml` as usual.
- **`choices`** — premade values you can cycle through with `↑`/`↓`
  while the field is focused. They're only suggestions: the field stays
  free text, so you can still type anything else. The choices are shown
  next to the field, with the current one highlighted.

Optional fields can't be removed with `Ctrl+D` (they belong to the
profile), and `Ctrl+N` resets every declared field back to its default.

## Running it

```
./tuirunner                              # restore last session from history.yaml
./tuirunner --profile default            # load a specific profile
./tuirunner VSS123 KVNR456               # fill the profile's must-have vars positionally
./tuirunner --profile default VSS123 KVNR456
```

- No arguments: reloads whatever profile, tab, script and variable
  values you had when you last quit.
- `--profile <name>`: loads that profile instead of the remembered one
  (errors out if the profile directory doesn't exist).
- Positional arguments: fill the active profile's `must_have` variables
  in the order they're listed in `vars.yaml`. These always win over
  whatever history had stored for those specific variables; anything
  else in the variable pool still comes from history.

There is no non-interactive mode — every invocation opens the TUI.

## Using the TUI

The box is inline in your terminal (not fullscreen) and sizes itself to
its content — it grows and shrinks with the number of scripts in the
current tab rather than stretching to fill the screen. On quit (however
you quit) the box is erased from the terminal rather than left sitting
in your scrollback. It's split into three stacked areas:

```
┌─ variables ───────────────────────────────┐
│ VSS*:        ...                           │
│ KVNR_LIST*:  ...                           │
├─ scripts ───────────────────────────────────┤
│ [connect]  curl   dbs   pods  ports  sql    │
│ ▸ ★ Prod-BE-Z1                              │
│     Prod-BE-Z2                              │
├─ preview ────────────────────────────────────┤
│ connect3 -e prod -d z1 -c be -g             │
│ ready - press Enter to run                  │
└──────────────────────────────────────────────┘
```

### Scripts area (middle)

| Key | Action |
|---|---|
| `←` / `→` | switch library tab |
| `↑` / `↓` | move through the script list |
| `Space` | toggle favorite (pins to top of the list, marked with `★`) |
| `/` | start typing to filter the current list; `Esc` clears and exits |
| `Enter` | run the selected script (see validation below) |
| `p` | open the profile switcher |
| `Ctrl+H` | open the run history (last 10 executions) — `↑`/`↓` choose, `Enter` restores that run's profile/tab/script/variables into the current session, `Esc` cancels |
| `Tab` | move focus into the variables area |
| `q` / `Ctrl+C` | quit immediately (session state is still saved) |
| `Esc`, `Esc` | quit (two taps; the first arms it and shows a hint, any other key cancels) |

### Variables area (top)

A shared pool of named values that persists as you move between
scripts and tabs — fill in `ENV` once, every script that uses `{{ENV}}`
picks it up. Fields for the profile's must-have variables are always
present, and so are its optional ones (pre-filled with their defaults);
other fields only appear once you explicitly add them.

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | move between fields (exits back to the scripts area at either end) |
| `Ctrl+R` | clear every field's value (fields stay) |
| `Ctrl+N` | new session — drop every field not declared in `vars.yaml`, and reset the declared ones to their defaults (blank if none) |
| `Ctrl+A` | add a new variable by name |
| `Ctrl+D` | delete the focused field (no-op on a must-have or optional field, or on an auto-derived `_LIST` field — see below) |
| `↑` / `↓` | on a field with `choices` in `vars.yaml`, cycle through them (typing free text still works) |
| `←` / `→` | on an auto-derived dropdown field, pick the previous/next value (on an ordinary text field, moves the cursor as usual) |

While you move through scripts, any variable name that the *currently
selected* script actually uses is highlighted (violet) so you can see
at a glance what it needs — this updates live as you change selection.

#### `_LIST` variables auto-derive two more fields

Any variable whose name ends in `_LIST` (must-have or ad-hoc, e.g. the
default profile's `KVNR_LIST`) automatically grows two extra, read/pick
-only fields the moment it exists, kept live as you edit it:

- **`<name without _LIST>`** — a dropdown over the comma-separated
  values (e.g. `KVNR_LIST = 1231,1231231,12312` gives a `KVNR` field you
  cycle through with `←`/`→`). Use this in a script wherever it needs
  exactly one of the values, e.g. `{{KVNR}}`.
- **`<name>_SQL`** — the same values rendered as a SQL `IN`-clause
  tuple, e.g. `KVNR_LIST_SQL` becomes `('1231','1231231','12312')`.
  Handy for `WHERE kvnr IN {{KVNR_LIST_SQL}}`.

Both are marked `(auto)`/shown dimmed in the input area, aren't directly
typeable, and disappear again if their source `_LIST` field is ever
removed (must-have `_LIST` fields can't be removed, so theirs are
permanent for the profile).

### Running a script

Press `Enter` on a script:

- If it references a `{{PLACEHOLDER}}` that has no matching field yet,
  you're asked whether to add it (`y` adds it and jumps you to fill it
  in; anything else cancels — nothing runs).
- Otherwise, if any must-have variable or any placeholder the script
  references is still empty, `Enter` does nothing — the preview area
  shows `blocked - missing: ...` so you know what's left.
- Once everything required is filled, `Enter` clears the box from your
  terminal, prints the exact command it's about to run (`$ ...`), then
  execs it directly against your terminal — interactive commands (SSH,
  `psql`, `oc rsh`, …) behave exactly as normal, and there's no "return
  to the picker" afterwards. This run is also added to the `Ctrl+H`
  history log.

The preview area always shows the command with your current values
substituted in; anything still unfilled is shown as the literal
`{{NAME}}` token rather than a blank, so you always know the true shape
of what's about to run.

### Switching profiles

Press `p` to open a list of every directory next to the binary that
looks like a profile (i.e. has a `libraries/` folder). Picking one
reloads its libraries, favorites and declared (must-have and optional)
variables from scratch, the latter at their defaults.
