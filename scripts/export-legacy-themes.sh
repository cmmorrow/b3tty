#!/usr/bin/env bash
#
# Exports the themes that were built into b3tty before 1.14.0 (Catppuccin,
# Dracula, Gruvbox Light, One Light, Solarized, Tokyo Night) as .b3theme
# packages, so people who used them can install them.
#
# Built-in themes can't be exported, so this script reads each theme's JSON
# from the last commit that had it in src/default_themes, adds it as a user
# theme to a scratch config, and runs 'b3tty theme export' on it. That gives
# the packages exactly the validation of any other export.
#
# The scratch config and HOME live in a state directory (default
# <out-dir>/.b3tty-export) so the export never touches your real config,
# plugins, or a running server. Keep that directory: the config holds each
# theme's package id, and re-running with the same state directory and a
# bumped --theme-version produces upgrades of the same packages rather than
# new, unrelated ones.
#
# Usage:
#   make build
#   scripts/export-legacy-themes.sh <out-dir> [options]
#
# Options:
#   --b3tty <path>          b3tty binary built from 1.14.0 or later (default ./b3tty)
#   --author <name>         package author (default: git config user.name)
#   --repo <https url>      repository the packages point to
#                           (default https://github.com/cmmorrow/b3tty)
#   --theme-version <ver>   version of the packages (default 1.0.0)
#   --ref <commit>          commit that still has the themes (default 4844ea0)
#   --state <dir>           state directory (default <out-dir>/.b3tty-export)

set -euo pipefail

usage() {
    sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
}

b3tty=./b3tty
author=$(git config user.name 2>/dev/null || true)
repo=https://github.com/cmmorrow/b3tty
version=1.0.0
ref=4844ea0
out=
state=

while [[ $# -gt 0 ]]; do
    case $1 in
        --b3tty) b3tty=$2; shift 2 ;;
        --author) author=$2; shift 2 ;;
        --repo) repo=$2; shift 2 ;;
        --theme-version) version=$2; shift 2 ;;
        --ref) ref=$2; shift 2 ;;
        --state) state=$2; shift 2 ;;
        -h|--help) usage ;;
        -*) echo "unknown option: $1" >&2; usage ;;
        *) [[ -z $out ]] || usage; out=$1; shift ;;
    esac
done
[[ -n $out ]] || usage
[[ -n $author ]] || { echo "no author: pass --author" >&2; exit 2; }
[[ -x $b3tty ]] || { echo "$b3tty is not an executable; run 'make build' or pass --b3tty" >&2; exit 2; }

# name|description, in the order they're exported.
themes=(
    "catppuccin-latte|Catppuccin Latte, the light flavor of the Catppuccin palette (catppuccin.com)."
    "catppuccin-mocha|Catppuccin Mocha, the darkest flavor of the Catppuccin palette (catppuccin.com)."
    "dracula|Dracula, the dark theme by Zeno Rocha (draculatheme.com)."
    "gruvbox-light|Gruvbox Light, the light variant of Pavel Pertsev's retro groove palette."
    "one-light|One Light, the light syntax theme from the Atom editor."
    "solarized-dark|Solarized Dark, Ethan Schoonover's precision palette on a dark background."
    "solarized-light|Solarized Light, Ethan Schoonover's precision palette on a light background."
    "tokyo-night|Tokyo Night, the dark theme inspired by Tokyo's city lights, by enkia."
)

mkdir -p "$out"
state=${state:-$out/.b3tty-export}
config=$state/.config/b3tty/conf.yaml

# Export never overwrites a file, so check every target before doing anything.
existing=()
for entry in "${themes[@]}"; do
    name=${entry%%|*}
    [[ ! -e $out/$name.b3theme ]] || existing+=("$out/$name.b3theme")
done
if [[ ${#existing[@]} -gt 0 ]]; then
    printf 'already exists, move it aside first: %s\n' "${existing[@]}" >&2
    exit 1
fi

# Seed the scratch config once; later runs reuse it to keep the package ids.
if [[ ! -f $config ]]; then
    mkdir -p "$(dirname "$config")"
    {
        echo "themes:"
        for entry in "${themes[@]}"; do
            name=${entry%%|*}
            echo "  $name:"
            # theme JSON (camelCase keys) -> config YAML (hyphenated keys).
            git show "$ref:src/default_themes/$name.json" | perl -ne '
                next unless /^\s*"(\w+)":\s*(".*?")\s*,?\s*$/;
                my ($k, $v) = ($1, $2);
                $k =~ s/([A-Z])/"-" . lc $1/ge;
                print "    $k: $v\n";
            '
        done
    } >"$config"
    echo "seeded $config from $ref"
fi

for entry in "${themes[@]}"; do
    name=${entry%%|*}
    HOME=$state "$b3tty" theme export "$name" \
        --output "$out/$name.b3theme" \
        --author "$author" \
        --repo "$repo" \
        --description "${entry#*|}" \
        --theme-version "$version"
done
