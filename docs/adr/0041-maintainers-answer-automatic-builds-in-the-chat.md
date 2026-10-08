# ADR-0041: Maintainers answer automatic builds in the chat

Date: 2026-10-08

Status: Accepted

## Context

[ADR-0040](0040-automatic-builds-run-one-subagent-for-each-ready-issue.md) lets the maintainer answer an automatic build in two places. A comment from the maintainer account without an agent marker is a requested change, and the maintainer answers a stop comment on the Issue and then sets the Project status. The maintainer also gives feedback in the chat of a running dispatcher, which records it in a `<!-- maintainer-feedback -->` comment.

The maintainer answers agents only in a chat session and does not write comments for agents on GitHub. Agents post with the account of the maintainer, so a comment of an agent without its marker looks like a request from the maintainer. Each run of the dispatcher starts without the chat of earlier runs, so an answer that stays only in a chat does not reach a later run.

## Decision

The maintainer answers automatic builds and asks for changes only in a chat session. The agent of that session records each answer and each request on GitHub. This decision replaces two parts of ADR-0040: the requested changes from a comment without a marker, and the status change by the maintainer after an answer.

A requested change is only a PR comment with the `<!-- maintainer-feedback -->` marker. The dispatcher reads only the comments from the account of the maintainer, and it does not act on a comment without an agent marker. A session that is not a dispatcher records feedback in the same way.

After a subagent makes a requested change and pushes it, it edits the feedback comment and adds the line `Done in <commit SHA>.` with a commit on the PR branch. The requested change stays open until this line names a commit that the PR branch contains. The subagent does not post a separate reply.

When the maintainer answers a stop in the chat, the agent of that session edits the stop comment and adds the line `Answer:` with the answer in English. Then it sets the Project status to `Todo` when the Issue has no open PR, or to `In Progress` when the PR is open. A later run reads the question and the answer in the same comment.

The report at the end of each dispatcher run states the question of each build that stopped, so that the maintainer can answer in the chat.

## Alternatives Considered

### Answers in GitHub comments

- Pros: the answer is on the Issue or the PR where the question is, without an agent step.
- Cons: the maintainer must leave the chat, and a comment without a marker from the shared account can come from an agent.
- Rejected: an answer in the chat keeps the work in one place, and only marked comments carry requests.

### A separate comment for each answer and each reply

- Pros: each event has its own comment and time.
- Cons: each stop and each requested change adds one more comment to read and to match with its question.
- Rejected: one edited comment keeps the question with its answer, and the request with its commit.

### Answers only in the chat, without a record on GitHub

- Pros: GitHub gets no extra comment text.
- Cons: a later run starts without the chat, so it cannot see the answer or the request.
- Rejected: the record on GitHub is the only state that a later run can read.

## Consequences

- An answer reaches a later run only after an agent records it, so the maintainer must answer in a session that can write to GitHub.
- A comment that the maintainer writes on GitHub without a marker has no effect on an automatic build.
- Agents edit the stop comment and the feedback comment after they post them, so GitHub shows them as edited.
- `.agents/commands/build.md`, the GitHub Issue conventions, and the Pull request conventions need these rules, and the Pull request conventions drop the requested change reply.
