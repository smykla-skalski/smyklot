ALTER TABLE pending_ci_intents
DROP CONSTRAINT pending_ci_intents_intent_kind_check;

ALTER TABLE pending_ci_intents
ADD CONSTRAINT pending_ci_intents_intent_kind_check
CHECK (intent_kind IN ('arm', 'cancel', 'draft'));
