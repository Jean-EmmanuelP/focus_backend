-- Challenge V2: Friend challenge features
-- invite_code, mantra, title, taunts

ALTER TABLE public.wake_up_challenges ADD COLUMN IF NOT EXISTS invite_code TEXT UNIQUE;
ALTER TABLE public.wake_up_challenges ADD COLUMN IF NOT EXISTS mantra TEXT;
ALTER TABLE public.wake_up_challenges ADD COLUMN IF NOT EXISTS title TEXT;

ALTER TABLE public.wake_up_entries ADD COLUMN IF NOT EXISTS mantra_validated BOOLEAN DEFAULT false;
ALTER TABLE public.wake_up_entries ADD COLUMN IF NOT EXISTS exercises_done BOOLEAN DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_wake_up_challenges_invite_code ON public.wake_up_challenges (invite_code) WHERE invite_code IS NOT NULL;

-- Taunts table
CREATE TABLE IF NOT EXISTS public.challenge_taunts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    challenge_id UUID NOT NULL REFERENCES public.wake_up_challenges(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_challenge_taunts_challenge_id ON public.challenge_taunts (challenge_id);
