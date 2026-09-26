# [P1] CJK-friendly emphasis and soft line breaks

Labels: P1, ai-native, i18n

## Problem

CommonMark's flanking rules make `**注意：**这是重点` render literally (`**` next to full-width punctuation followed by a CJK letter is not right-flanking). It is spec-correct and the single most common rendering defect in Chinese/Japanese model output. Separately, a soft break between two CJK characters renders as a space.

## Proposal

- An opt-in extension implementing the CJK-friendly emphasis amendment (as in `markdown-it-cjk-friendly`): treat CJK characters and CJK punctuation as neither whitespace nor punctuation for flanking purposes.
- An opt-in soft-break policy that drops the break between two CJK characters.
- Include both in the LLM profile (see the LLM profile issue).

## Acceptance

- Golden tests from the amendment's examples; default profile stays 652/652.
