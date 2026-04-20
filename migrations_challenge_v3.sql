-- Challenge V3: Challenge type support
-- Stores what kind of challenge this is (wakeup, gym, meditation, reading, custom)

ALTER TABLE public.wake_up_challenges ADD COLUMN IF NOT EXISTS challenge_type TEXT DEFAULT 'wakeup';
