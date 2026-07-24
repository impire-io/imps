# Episode template

Copy for the next-numbered episode (`NNNN-short-kebab-slug.md`). Every field
below is part of the record; **Reversal condition** and evidence-class tags are
required on every episode (the structural lint checks for the
Reversal-condition line). After writing the episode, add it to the index in
[`README.md`](README.md) and refresh "Where things stand" there.

````markdown
# Episode NNNN — <title> (<dates>)

<What happened, in a few sentences: the question, what was built/measured, the
honest outcome with key numbers. Tag load-bearing claims with their evidence
class: [measured] / [mechanism-argument] / [judgment] — only measured closes a
debate. For a framework, [measured] means a reading in the repo: a test, a
benchmark, a compile-deny outcome, a byte-diff.>

<What was refuted or reversed, if anything.>

<What it taught / what it opened.>

Reversal condition: <for direction decisions: what evidence would change our
minds, written now, phrased as an observable reading. For features with no
direction component: "none — records a completed build/measurement".>

Trail: <docs>; commits <hashes>.
````
