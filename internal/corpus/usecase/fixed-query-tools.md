---
group: Read paths
description: public audience, a known set of questions
---
# Fixed query tools

A `Toolset` of zero-parameter queries. The read path for a public audience.

**Seen in:** a public catalogue widget whose tools cover products,
categories, locations, recommendations and downloads.

**Checked:** against every tool in one Toolset - all of them carry a zero-parameter inputSchema - and the CRD. What a tool manifest hands the model against asgard-core `478cf5d6` `internal/bpcontroller/server/bp_controller.go`; the absence of `Toolset.spec.instruction` against asgard-kube `3da0365` `crd/asgard-ai.com_toolsets.yaml`.

**Unchecked:** whether the description guidance changes which tool a model picks. That shows only against a running agent.

Read the platform side first: `../wiki/semantic-model.md` -
what a Semantic Model is, how it is built, and its limits. This page assumes you have.

## When this shape, and when not

Use it when the audience is anonymous. What can be asked is then decided by a
few statements in version control, no user input reaches SQL, and widening it
takes a CR change and a review.

Do not use it for an internal audience with open-ended questions - you will
end up writing a tool per question. A semantic layer is the right shape there.

The agent can only answer what the queries return, and that limit is
deliberate.

## The shape

    DataConnector  dc-<system>
      <- Workflow  wf-<tool>     one per tool, a query-database processor
      <- Toolset   ts-<name>     tools[] -> (workflow, entry)
           <- SandboxBlueprint.toolsetNames

One Workflow per tool, each with its own entry name. The Toolset points at
`(workflow, entry)` pairs.

## Generate it

    asgard-cli add querytool <name> --connector dc-<name> --toolset ts-<name>

That writes the structure below with the fields that fail silently already in
place - the display annotation, the labels the UI needs, the current field names.
When the skeleton is copied by hand these get lost, and nothing reports them
missing: not helm lint, not CRD validation, not a server dry-run.

The generated file marks the judgement calls TODO. The rest of this page covers
them.

## The skeleton

One Workflow per tool in `templates/tool/wf-<name>.yaml`, plus one Toolset in
`templates/toolset/ts-<name>.yaml`.

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: Workflow
metadata:
  name: wf-<tool>
  annotations:
    asgard-ai.com/workflow-name: "<display name>"
    {{- include "<chart>.workflowSetAnnotations" (dict "name" "wf-<tool>" "displayName" "<display name>") | nindent 4 }}
  labels:
    {{- include "<chart>.workflowSetLabels" (dict "name" "wf-<tool>" "type" "automation_tool") | nindent 4 }}
    {{- with .Values.asgard.projectEnvironmentId }}
    asgard-ai.com/project-environment-id: {{ . | quote }}
    {{- end }}
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  variables: []
  entries:
    - name: entry-main
      labels:
        display_name: Entry
        description: <what calling this does>
      handlingProcessor: proc-query
      inputSchema: |-
        {
          "type": "object",
          "properties": {}
        }
      tooling:
        # This is where tool usage guidance lives - Toolset has no instruction field.
        name: <snake_case_tool_name>
        description: |-
          <what it returns, how many rows, and when the agent should reach for it>
        allowUploadFile: false
  exits: []
  processors:
    - name: proc-query
      type: query-database
      labels:
        display_name: SQL
        description: <what the query does>
      configs:
        - name: dataConnector
          value: dc-<system>
        # REQUIRED in practice: a MISSING allowWrite resolves to true, and the
        # definitions describe it as permitting INSERT, UPDATE, DELETE and DDL.
        # This shape serves anonymous callers - see `../wiki/processors.md`.
        - name: allowWrite
          value: "false"
        - name: resultField
          value: <field>
        - name: sql
          value: |-
            select ...
    # The query needs somewhere to come out: `resultField` names the variable
    # the rows land in, and this returns them. Every deployed fixed query tool
    # has this pair.
    - name: proc-response
      type: push-message
      labels:
        display_name: Response
      configs:
        - name: payload
          expression: |-
            (() => {
              return <field>;
            })()
  # **Without this the run stops at proc-query.** The CR is legal, the run
  # succeeds, and the tool answers nothing - `gate` W3 is what reports it.
  relationships:
    - from: {processor: proc-query, relationName: success}
      to: {processor: proc-response}
---
apiVersion: asgard-ai.com/v1alpha1
kind: Toolset
metadata:
  name: ts-<name>
  annotations:
    asgard-ai.com/toolset-name: "<display name>"
    asgard-ai.com/toolset-description: "<what this set covers>"
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  toolsetClass: workflow-tooling
  apiKey:
    valueFrom:
      secretKeyRef:
        name: {{ include "<chart>.appSecretName" . }}
        key: asgard_resource_api_key
  tools:
    - entrypoint:
        entry: entry-main
        workflow: wf-<tool>
      requestConsent: false
```

Put the reasoning - why this projection, which soft-delete filter, why a `COUNT`
is `DISTINCT` - in the file's header comment. There is nowhere else for it.

## Zero parameters

Every tool takes no arguments, so no user input ever reaches SQL and the
injection surface is zero.

The entry provides that, not the field. `sql` is a config like every
other, so it takes a `value`, an `expression` or a `template` - string-building
a query out of the payload is possible, and it is the hole this shape
closes. Where a query genuinely needs a parameter, it is bound with
`sql.args.<n>.type` and `sql.args.<n>.value`, never written into the SQL text -
"Parameterised SQL in `query-database`" in `../wiki/processors.md` has the
numbering, each database's placeholder and an example. Binding removes
injection and leaves exposure: a public caller still chooses which rows a
parameter selects, which is why this shape takes none.

A query that wants a keyword filter gets rewritten to aggregate instead. One that
filtered with `ILIKE '%keyword%'` became a `string_agg` returning one row per
product, and the agent picks from the result.

`requestConsent: false` on all of them - they are read-only.

## Do not ship two tools that differ only in projection

Two tools on the same FROM/JOIN differing only in which columns they return force
each one's `description` to name the other as the alternative, and that is
where a model picks wrong.


Merge them, and make the difference a column value instead of a tool choice.
"Do I want the empty categories?" becomes `product_count = 0` rather than a
second tool. One deployment merged tools together this way, and recorded that
the merge also settled a real inconsistency: the two queries had used
different join types and different soft-delete filters, so they disagreed about
what existed.

## Designing the parts the generator leaves TODO

### Which queries become tools

Start from what the audience actually asks, not from what the database can
answer. For a public catalogue that is a short list: what do you sell, how are
they grouped, where are you, what fits my situation, where do I download the
spec. Five questions, five tools.

A useful test: could a person answer this from one screen of the website? If
yes it is probably a tool. If it needs judgement or comparison, it belongs to a
knowledge source instead.

Resist one tool per column. The set should be small enough that the model can
hold all of them, and each one distinct enough that choosing between them is
obvious.

### Writing `tooling.description`

The model picks a tool from its `tooling.name`, this text and its inputSchema -
the only parts of a tool manifest that describe the tool - and a zero-parameter inputSchema says
nothing, so this text carries three things:

    <what it returns>, <how many rows>, and <when to reach for it>

    列出全部 10 個服務據點的名稱、地址、電話。
    客戶問「你們在哪裡有據點」「花蓮有沒有」「電話幾號」時用這支。

Say the row count, measured. It tells the model whether to expect a
complete list or a sample, and it tells the next reader whether the query still
does what it claims.

Say what the tool does not do. A zero-parameter tool returns everything and
the model filters afterwards - if that is not said, it will wait for a filter
parameter that does not exist, or call the tool repeatedly hoping for different
results.

Say what an empty result means. "No rows" is not "we do not sell it" unless
you say so, and the difference is a wrong answer to a customer either way.

What is true of several of these tools goes in the skill instead, where it
is written once rather than in every description that could carry it -
`../wiki/tool-description-and-skill.md`, which is read before both halves are
written, because the model reads them together.

## Fields that are not obvious

`Toolset.spec.instruction` does not exist. It was removed from the CRD. Tool
usage guidance lives in each tool's Workflow, in
`entries[].tooling.description`.

Do not add it back: the CRD silently prunes it, `kubectl apply
--dry-run=server` reports success, and then helm's server-side apply fails the
deploy with `field not declared in schema`. That cost a broken release once,
after passing every dry run.

The 口徑 lives in the SQL now. With no `instruction` field, anything a reader
needs to know - soft deletes, how a category path is built, why a `COUNT` is
`DISTINCT` - goes in the header comment of the tool's file.

Measure the row counts and write them down. A tool's description saying "89
products" sets the agent's expectations; a stale number is worse than none, so
note when it was measured.

Every tool workflow needs the full workflow-set label set, with
`workflow-set-type: automation_tool` - that is also what a Toolset's tool picker
filters on.

## Verify

```bash
# run each tool's SQL against the real database and record the row count
.venv/bin/python .agents/skills/db-query/scripts/query.py \
  --class <class> --prefix <PREFIX> -f tool.sql

asgard-cli gate               # every local check, the lint step included
asgard-cli verify <project>   # or one step alone, while iterating
```

Never run `helm lint` by hand: without the reserved `asgard` values file
that `gate` supplies, every chart that labels anything fails. `asgard-cli gate
--help` says why.

The xref check resolves every `(workflow, entry)` pair. A wrong `entry` name is
as fatal as a wrong workflow name and apply catches neither, so both halves are
checked.
