#!/usr/bin/env python3
"""Verify the bundled skill's recipes against synthetic Markdown, using only stdlib."""

import argparse
import csv
import io
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import tempfile
from urllib.parse import unquote, urlsplit


REPOSITORY = Path(__file__).resolve().parents[1]
DEFAULT_SKILL_DIR = REPOSITORY / "skills/mdsql"
LINES = [
    "---",
    "owner: Ada",
    "status: active",
    "categories: ['[[Agent Tasks]]', '[[Projects]]', '[[Agent Tasks]]']",
    "meta: {stage: plan, count: 2}",
    "---",
    "# Project",
    "Preamble.",
    "## Examples",
    "- [ ] Prepare #work",
    "- [/] Review #work/team",
    "- [x] Finished #work",
    "- [-] Cancelled #work",
    "",
    "```go",
    'fmt.Println("sample")',
    "```",
    "",
    "| Name | Workflow |",
    "| --- | --- |",
    "| sample | Build \\| verify |",
    "",
    "[External](https://example.org)",
    "",
    "## Next",
    "Next body.",
]


def heading_anchors(content):
    """Return GitHub-style anchors for the simple headings used by this skill."""
    anchors = set()
    counts = {}
    for heading in re.findall(r"^ {0,3}#{1,6}\s+(.+?)\s*#*\s*$", content, re.MULTILINE):
        heading = re.sub(r"[`*_~]", "", heading)
        anchor = re.sub(r"[^\w -]", "", heading.lower()).replace(" ", "-")
        occurrence = counts.get(anchor, 0)
        counts[anchor] = occurrence + 1
        if occurrence:
            anchor = f"{anchor}-{occurrence}"
        anchors.add(anchor)
    return anchors


def verify_internal_links(skill_dir):
    skill_dir = skill_dir.resolve()
    markdown_files = sorted(skill_dir.rglob("*.md"))
    assert markdown_files, f"No Markdown files found under {skill_dir}"
    for source in markdown_files:
        content = source.read_text()
        for raw_destination in re.findall(r"(?<!!)\[[^]]*\]\(([^)]+)\)", content):
            destination = raw_destination.strip().strip("<>")
            parsed = urlsplit(destination)
            if parsed.scheme or parsed.netloc:
                continue
            relative_path = unquote(parsed.path)
            assert not Path(relative_path).is_absolute(), (
                f"Absolute link is not portable: {source}: {destination}"
            )
            target = (source.parent / relative_path).resolve() if relative_path else source.resolve()
            assert target == skill_dir or skill_dir in target.parents, (
                f"Link escapes skill directory: {source}: {destination}"
            )
            assert target.is_file(), f"Missing skill link target: {source}: {destination}"
            if parsed.fragment:
                anchor = unquote(parsed.fragment)
                assert anchor in heading_anchors(target.read_text()), (
                    f"Missing skill anchor: {source}: {destination}"
                )


def load_recipes(skill_dir):
    skill = skill_dir / "SKILL.md"
    content = skill.read_text()
    size = len(content.encode("utf-8"))
    assert size <= 7000, f"SKILL.md has {size} bytes; limit is 7000"
    verify_internal_links(skill_dir)
    recipes = []
    for block in re.findall(r"```bash\n(.*?)\n```", content, re.DOTALL):
        for line in block.splitlines():
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            arguments = shlex.split(line)
            assert arguments[0] == "mdsql", f"Unexpected recipe executable: {line}"
            assert arguments[-1] in {"example.md", "./notes/project"}, (
                f"Recipe must use the synthetic example scopes: {line}"
            )
            recipes.append(arguments[1:])
    assert len(recipes) == 10, "Update semantic expectations when changing the recipe set"
    return recipes, size


def csv_rows(text):
    return list(csv.DictReader(io.StringIO(text)))


def verify(outputs):
    project_file = "notes/project/a.md"
    assert csv_rows(outputs[0]) == [
        {"file": project_file, "line": "10", "text": "Prepare #work", "state": "todo"},
        {"file": project_file, "line": "11", "text": "Review #work/team", "state": "in_progress"},
    ], "Active-task recipe selected the wrong tasks"
    assert csv_rows(outputs[1]) == [
        {"file": project_file}, {"file": "notes/project/b.md"}
    ], "Scalar/list membership must return each matching file once"
    assert csv_rows(outputs[2]) == [
        {"file": project_file, "line": "10", "text": "Prepare #work"},
        {"file": project_file, "line": "11", "text": "Review #work/team"},
        {"file": project_file, "line": "12", "text": "Finished #work"},
        {"file": project_file, "line": "13", "text": "Cancelled #work"},
    ], "Subquery recipe selected tasks from the wrong files"
    assert csv_rows(outputs[3]) == [
        {"file": "example.md", "table": "1", "row": "1", "value": "Build | verify"}
    ], "Cell recipe must exclude headers and preserve escaped pipes"
    assert csv_rows(outputs[4]) == [
        {"file": "example.md", "n": str(n), "text": LINES[n - 1]}
        for n in range(20, 26)
    ], "Line recipe returned the wrong interval"
    assert csv_rows(outputs[5]) == [
        {"file": project_file, "n": "26", "text": "Next body."},
        {"file": project_file, "n": "25", "text": "## Next"},
        {"file": "notes/project/b.md", "n": "5", "text": "# Other"},
        {"file": "notes/project/b.md", "n": "4", "text": "---"},
    ], "Per-file line recipe did not keep the last two lines of each file"
    assert csv_rows(outputs[6]) == [
        {"file": "example.md", "start": str(start), "end": str(end),
         "raw": "\n".join(LINES[start - 1:end])}
        for start, end in [(9, 24), (25, 26)]
    ], "Section recipe lost source ranges or content"
    assert outputs[7] == (
        'line\ttext\n@"notes/project/a.md"\n'
        "10\tPrepare #work\n11\tReview #work/team\n"
        "12\tFinished #work\n13\tCancelled #work\n"
    ), "Grouped recipe changed columns, attribution, or rows"
    assert json.loads(outputs[8]) == [
        {"file": project_file, "fm": {
            "owner": "Ada", "status": "active",
            "categories": ["[[Agent Tasks]]", "[[Projects]]", "[[Agent Tasks]]"],
            "meta": {"stage": "plan", "count": 2},
        }},
        {"file": "notes/project/b.md", "fm": {
            "owner": "Bob", "categories": "[[Agent Tasks]]"
        }},
    ], "JSON recipe lost nested metadata or types"
    assert csv_rows(outputs[9]) == [
        {"lang": "go", "start": "15", "end": "17", "code": 'fmt.Println("sample")'}
    ], "Combined code filters selected the wrong block"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, help="Built mdsql executable or command on PATH")
    parser.add_argument(
        "--skill-dir", type=Path, default=DEFAULT_SKILL_DIR,
        help="Skill payload to validate (defaults to the bundled skills/mdsql)",
    )
    options = parser.parse_args()
    binary = shutil.which(options.binary)
    if binary is None:
        parser.error(f"Executable not found: {options.binary}")
    binary = str(Path(binary).resolve())
    skill_dir = options.skill_dir.resolve()
    recipes, size = load_recipes(skill_dir)
    with tempfile.TemporaryDirectory(prefix="mdsql-skill-recipes-") as directory:
        root = Path(directory)
        project = root / "notes/project"
        project.mkdir(parents=True)
        document = "\n".join(LINES)
        (root / "example.md").write_text(document)
        (project / "a.md").write_text(document)
        (project / "b.md").write_text(
            '---\nowner: Bob\ncategories: "[[Agent Tasks]]"\n---\n# Other\n'
        )
        environment = dict(os.environ, MDSQL_CACHE="0", XDG_CACHE_HOME=str(root / "cache"))
        outputs = []
        for arguments in recipes:
            result = subprocess.run(
                [binary, *arguments], cwd=root, env=environment, input="",
                capture_output=True, text=True, timeout=15, check=False,
            )
            assert result.returncode == 0 and not result.stderr, (
                f"Recipe failed: {shlex.join(arguments)}\n"
                f"exit={result.returncode}\n{result.stderr}"
            )
            outputs.append(result.stdout)
        verify(outputs)
    print(f"PASS: {len(recipes)} skill recipes, internal links, SKILL.md {size}/7000 bytes")


if __name__ == "__main__":
    main()
