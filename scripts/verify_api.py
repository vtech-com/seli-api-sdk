#!/usr/bin/env python3
"""Compare what api/openapi.json promises against what the API actually serves.

Every guard in this repo checks the CLI against a document. Nothing in `go test`
checks the document against the API, and the document is the API's own
hand-maintained file: it can drift from the routes beside it.

This script asks the server instead. For every GET the spec declares that needs
no path parameter, it calls the endpoint and compares the response with the
200 response schema:

    MISSING   the schema requires a property the response did not carry
    EXTRA     the response carried a property the schema never declared
    STATUS    the path answered with something other than 200

EXTRA is the column to read. A field the schema omits is a field this CLI's help
pages will omit too, and a caller cannot see what was never printed.

A by-id GET (`/api/members/{id}`, ...) is called with an id taken from the
collection of the same prefix, when there is one. A GET whose path parameter is
not an id (`/api/delivery/{code}`) is listed as skipped: the script cannot
invent a waybill code.

With --cli, it also checks that each wrapped command's help OUTPUT section names
every field the API actually returned. A field absent from the sample is a
field the reader does not know exists.

Configuration is the environment, never the source:

    SELI_API_URL   the API host only, https (or localhost); paths are the spec's
                   own, `/api/...`, appended verbatim
    SELI_API_KEY   an ssk_ key; sent as a bearer token, never printed
    SELI_TENANT    the tenant code sent as X-Tenant-Code

Usage:
    make verify-api                      # builds ./dist/seli, then runs this
    python3 scripts/verify_api.py --cli dist/seli

Exit 0 when every endpoint matches its schema and every checked help page names
every field; 1 otherwise. Read-only: it makes GET requests only, creates
nothing, and the one local command it runs besides `--help` is `seli version`.
"""

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request

REPO_SPEC = os.path.join(os.path.dirname(__file__), "..", "api", "openapi.json")

# Each wrapped command whose help page shows a response: (spec path, argv). The
# page's OUTPUT section must name every field the API returns for that path. A
# list page is checked against the collection's rows, a get page against the
# by-id record. This is the one place the script learns which commands wrap
# which operation; keep it in step with implementedOps in
# cmd/openapi_path_coverage_test.go.
HELP_SAMPLES = [
    ("/api/health", ["health"]),
    ("/api/members", ["members", "list"]),
    ("/api/members/{id}", ["members", "get"]),
]

# Commands that answer without the API. Their page can be checked by running
# them: the fields they print must be the fields their sample shows.
LOCAL_CHECKS = [["version"]]

# Page-level sentinel for a section header, as the help skeleton defines them.
OUTPUT_HEADER = "OUTPUT"


def parse_args():
    p = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    p.add_argument("--base-url", default=os.environ.get("SELI_API_URL", ""),
                   help="the API host only (default: $SELI_API_URL)")
    # The key is read from the environment by default. Passing --key puts it in
    # the process list for the length of the run; prefer SELI_API_KEY.
    p.add_argument("--key", default=os.environ.get("SELI_API_KEY", ""),
                   help="API key (default: $SELI_API_KEY; prefer the environment)")
    p.add_argument("--tenant", default=os.environ.get("SELI_TENANT", ""),
                   help="tenant code (default: $SELI_TENANT)")
    p.add_argument("--spec", default=REPO_SPEC)
    p.add_argument("--cli", default="",
                   help="path to a built seli binary; also checks that each help page's "
                        "OUTPUT section names every field the API actually returns")
    p.add_argument("--timeout", type=int, default=30)
    args = p.parse_args()
    missing = [n for n, v in (("--base-url", args.base_url), ("--key", args.key),
                              ("--tenant", args.tenant)) if not v]
    if missing:
        p.error("missing " + ", ".join(missing) +
                " (or set SELI_API_URL / SELI_API_KEY / SELI_TENANT)")
    return args


class NoRedirect(urllib.request.HTTPRedirectHandler):
    """Never follow a redirect: urllib would forward the Authorization header."""

    def redirect_request(self, *a, **kw):
        return None


class Client:
    def __init__(self, base_url, key, tenant, timeout):
        self.base_url = base_url.rstrip("/")
        self.key, self.tenant, self.timeout = key, tenant, timeout
        self.opener = urllib.request.build_opener(NoRedirect)
        self.cache = {}

    def check_transport(self):
        """Refuse to send the key in clear text, as the CLI itself does."""
        u = urllib.parse.urlparse(self.base_url)
        if u.scheme != "https" and u.hostname not in ("localhost", "127.0.0.1"):
            sys.exit("verify_api: refusing a non-HTTPS API host other than localhost: "
                     "the key would travel in clear text")

    def get(self, path, with_tenant=True):
        """GET base+path; returns (status, parsed JSON or None). Cached per path."""
        if path in self.cache:
            return self.cache[path]
        req = urllib.request.Request(self.base_url + path, method="GET")
        req.add_header("Authorization", "Bearer " + self.key)
        req.add_header("Accept", "application/json")
        if with_tenant:
            req.add_header("X-Tenant-Code", self.tenant)
        try:
            with self.opener.open(req, timeout=self.timeout) as r:
                status, raw = r.status, r.read()
        except urllib.error.HTTPError as e:
            status, raw = e.code, e.read()
        except Exception as e:
            # The exception text can name the host, never the header.
            self.cache[path] = (0, str(e))
            return self.cache[path]
        try:
            body = json.loads(raw or b"null")
        except ValueError:
            body = None
        self.cache[path] = (status, body)
        return self.cache[path]


def resolve(spec, node, depth=0):
    """Follow $ref and flatten allOf, so a composed schema still lists its properties."""
    while isinstance(node, dict) and ("$ref" in node or "allOf" in node) and depth < 8:
        depth += 1
        if "$ref" in node:
            node = spec["components"]["schemas"].get(node["$ref"].rsplit("/", 1)[-1], {})
            continue
        merged = {"type": "object", "properties": {}, "required": []}
        for part in node["allOf"]:
            part = resolve(spec, part, depth)
            merged["properties"].update(part.get("properties", {}))
            merged["required"] += part.get("required", [])
        node = merged
    return node


def resolve_param(spec, p):
    if "$ref" in p:
        return spec["components"].get("parameters", {}).get(p["$ref"].rsplit("/", 1)[-1], {})
    return p


def response_schema(spec, op):
    try:
        return op["responses"]["200"]["content"]["application/json"]["schema"]
    except KeyError:
        return None


def diff(spec, schema, value, path=""):
    """(missing, extra) property paths of value against schema.

    missing: required by the schema, absent from the value.
    extra:   present in the value, never declared by the schema.
    Arrays are walked row by row (the first 100), objects key by key.
    """
    schema = resolve(spec, schema or {})
    missing, extra = set(), set()
    if isinstance(value, list):
        items = schema.get("items", {})
        for row in value[:100]:
            m, e = diff(spec, items, row, path)
            missing |= m
            extra |= e
        return missing, extra
    if not isinstance(value, dict):
        return missing, extra
    props = schema.get("properties")
    if props is None:
        return missing, extra  # an open object: nothing declared, nothing to compare
    for name in schema.get("required", []):
        if name not in value:
            missing.add(f"{path}{name}")
    for name, v in value.items():
        if name not in props:
            extra.add(f"{path}{name}")
        elif v is not None:
            m, e = diff(spec, props[name], v, f"{path}{name}.")
            missing |= m
            extra |= e
    return missing, extra


def keys_of(value, path=""):
    """Every key path in a response, descending through objects and arrays."""
    out = set()
    if isinstance(value, list):
        for row in value[:100]:
            out |= keys_of(row, path)
    elif isinstance(value, dict):
        for k, v in value.items():
            out.add(f"{path}{k}")
            out |= keys_of(v, f"{path}{k}.")
    return out


def has_record(body):
    d = body.get("data") if isinstance(body, dict) else None
    return bool(d) if isinstance(d, (list, dict)) else False


def first_record(body):
    if not isinstance(body, dict):
        return None
    d = body.get("data")
    if isinstance(d, list):
        return d[0] if d else None
    return d if isinstance(d, dict) else None


def required_unsuppliable(spec, op):
    """Required query parameters this script cannot invent a value for."""
    return [p["name"] for p in (resolve_param(spec, q) for q in op.get("parameters", []))
            if p.get("required") and p.get("in") == "query"]


def wants_tenant(spec, op):
    return any(resolve_param(spec, p).get("name", "").lower() == "x-tenant-code"
               for p in op.get("parameters", []))


def judge(spec, op, status, body):
    """(note, fails) for one endpoint."""
    if status == 0:
        return f"STATUS unreachable: {body}", True
    if status != 200:
        code = ""
        if isinstance(body, dict) and isinstance(body.get("error"), dict):
            code = " " + str(body["error"].get("code", ""))
        return f"STATUS {status}{code}", True
    if not isinstance(body, dict):
        return "STATUS 200 but the body is not a JSON object", True
    schema = response_schema(spec, op)
    if schema is None:
        return "spec declares no 200 JSON schema", False
    missing, extra = diff(spec, schema, body)
    notes = []
    if missing:
        notes.append("MISSING " + ",".join(sorted(missing)))
    if extra:
        notes.append("EXTRA " + ",".join(sorted(extra)))
    if not has_record(body):
        notes.append("no record returned (empty tenant?), fields not compared")
    return ("  ".join(notes) or "match"), bool(missing)


def output_section(page):
    """The OUTPUT section of a help page: from its header line to the end."""
    lines = page.split("\n")
    for i, line in enumerate(lines):
        if line == OUTPUT_HEADER:
            return "\n".join(lines[i + 1:])
    return ""


def help_output(cli, argv):
    run = subprocess.run([cli] + argv + ["--help"], capture_output=True, text=True)
    out = output_section(run.stdout)

    # A page may defer to another page's sample ("the same shape as members get")
    # rather than copy it: a fact true of two pages is stated on one. Follow the
    # pointer. The phrase wraps across lines in a page, so match flattened text.
    flat = " ".join(out.split())
    m = re.search(r"same shape as ([a-z][a-z-]*(?: [a-z][a-z-]*)?) get", flat)
    if m:
        target = m.group(1).split() + ["get"]
        sub = subprocess.run([cli] + target + ["--help"], capture_output=True, text=True)
        out += "\n" + output_section(sub.stdout)
    return out


def names_all(label, out, fields):
    """Print and return 1 when the OUTPUT section omits a field the API returned."""
    # A field counts as named when it appears as a JSON key: "field".
    omitted = sorted(f for f in fields if f'"{f.rsplit(".", 1)[-1]}"' not in out)
    if not out:
        print(f"  {label:<22} SAMPLE OMITS everything: no OUTPUT section on the page")
        return 1
    if omitted:
        print(f"  {label:<22} SAMPLE OMITS " + ",".join(omitted))
        return 1
    print(f"  {label:<22} names all {len(fields)} fields")
    return 0


def check_help_samples(c, cli, spec, ids):
    print("\nHelp OUTPUT samples vs the API's own fields:")
    failures = 0
    for path, argv in HELP_SAMPLES:
        label = " ".join(argv)
        concrete = path
        if "{id}" in path:
            base = path.split("/{")[0]
            if base not in ids:
                print(f"  {label:<22} skipped: no record to take an id from")
                continue
            concrete = path.replace("{id}", urllib.parse.quote(ids[base], safe=""))
        status, body = c.get(concrete)
        if status != 200 or not isinstance(body, dict):
            print(f"  {label:<22} skipped: endpoint answered {status}")
            continue
        if not has_record(body):
            print(f"  {label:<22} skipped: no record to compare against")
            continue
        # data fields, then the API's own meta (pagination and whatever else it
        # sends): the CLI prints both, so the page must name both.
        fields = keys_of(body.get("data"))
        fields |= {f"meta.{k}" for k in keys_of(body.get("meta") or {})}
        failures += names_all(label, help_output(cli, argv), fields)
    return failures


def check_local_samples(cli):
    failures = 0
    home = tempfile.mkdtemp(prefix="seli-verify-")
    # A throwaway HOME and no SELI_* variables: the local commands read neither
    # the operator's config nor their key.
    env = {"HOME": home, "USERPROFILE": home, "PATH": os.environ.get("PATH", "")}
    for argv in LOCAL_CHECKS:
        label = " ".join(argv)
        run = subprocess.run([cli] + argv, capture_output=True, text=True, env=env)
        try:
            doc = json.loads(run.stdout)
        except ValueError:
            print(f"  {label:<22} SKIPPED: did not print JSON (exit {run.returncode})")
            failures += 1
            continue
        if "error" in doc:
            # A failed command printed an error object, not a record. Counting its
            # zero fields as "names all 0" is the false pass this script exists
            # to prevent.
            print(f"  {label:<22} SKIPPED: command failed (exit {run.returncode})")
            failures += 1
            continue
        fields = keys_of(doc.get("data"))
        failures += names_all(label, help_output(cli, argv), fields)
    return failures


def main():
    args = parse_args()
    with open(args.spec) as f:
        spec = json.load(f)
    c = Client(args.base_url, args.key, args.tenant, args.timeout)
    c.check_transport()

    print(f"base={c.base_url}  tenant={c.tenant}  spec={os.path.relpath(args.spec)}\n")

    rows, failures, skipped = [], 0, []
    ids = {}  # collection path -> the id of its first record

    gets = sorted((p, v["get"]) for p, v in spec["paths"].items() if "get" in v)
    collections = [(p, op) for p, op in gets if "{" not in p]
    by_id = [(p, op) for p, op in gets if "{" in p]

    for path, op in collections:
        if needed := required_unsuppliable(spec, op):
            skipped.append((path, "requires query " + ",".join(needed)))
            continue
        status, body = c.get(path, with_tenant=wants_tenant(spec, op))
        note, bad = judge(spec, op, status, body)
        rows.append((path, status, note))
        failures += bad
        rec = first_record(body)
        if isinstance(rec, dict) and rec.get("id"):
            ids[path] = str(rec["id"])

    for path, op in by_id:
        params = re.findall(r"\{(\w+)\}", path)
        if params != ["id"]:
            skipped.append((path, "needs " + ",".join(params) + ", which this script cannot invent"))
            continue
        base = path.split("/{")[0]
        if base not in ids:
            skipped.append((path, f"no record in {base} to take an id from"))
            continue
        if needed := required_unsuppliable(spec, op):
            skipped.append((path, "requires query " + ",".join(needed)))
            continue
        concrete = path.replace("{id}", urllib.parse.quote(ids[base], safe=""))
        status, body = c.get(concrete, with_tenant=wants_tenant(spec, op))
        note, bad = judge(spec, op, status, body)
        rows.append((path, status, note))
        failures += bad

    width = max([len(p) for p, _, _ in rows] + [len(p) for p, _ in skipped] + [1])
    for path, status, note in rows:
        code = "  -" if status == 0 else f"{status:>3}"
        print(f"{path:<{width}}  {code}  {note}")
    for path, why in skipped:
        print(f"{path:<{width}}    -  skipped: {why}")

    if args.cli:
        print("\nLocal command samples vs the fields they print:")
        failures += check_local_samples(args.cli)
        failures += check_help_samples(c, args.cli, spec, ids)

    extra = sum(1 for _, _, note in rows if "EXTRA" in note)
    print()
    if extra:
        print(f"{extra} endpoint(s) return fields api/openapi.json never declares.")
        print("That is the document's defect, not the CLI's. Report it; do not let a help page inherit it.")
    if failures:
        print(f"\n{failures} failure(s): a page, or the spec, describes something the server does not do.")
        return 1
    print("\nEvery checked endpoint matches its schema and every checked page names every field it returns.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
