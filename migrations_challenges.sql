-- ==========================================
-- WAKE-UP CHALLENGE - Social alarm challenge
-- ==========================================

CREATE TABLE IF NOT EXISTS public.wake_up_challenges (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    creator_id uuid NOT NULL REFERENCES auth.users ON DELETE CASCADE,
    opponent_id uuid REFERENCES auth.users ON DELETE SET NULL,
    alarm_time text NOT NULL DEFAULT '07:00',          -- HH:mm format
    duration_days int NOT NULL DEFAULT 30,
    status text NOT NULL DEFAULT 'pending',             -- pending, active, completed, cancelled
    creator_streak int NOT NULL DEFAULT 0,
    opponent_streak int NOT NULL DEFAULT 0,
    creator_score int NOT NULL DEFAULT 0,               -- total days on time
    opponent_score int NOT NULL DEFAULT 0,
    start_date date,
    end_date date,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.wake_up_entries (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    challenge_id uuid NOT NULL REFERENCES public.wake_up_challenges ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES auth.users ON DELETE CASCADE,
    day_number int NOT NULL,                            -- 1 to 30
    wake_up_time text,                                  -- HH:mm actual wake up
    photo_url text,                                     -- Supabase Storage path
    is_on_time boolean NOT NULL DEFAULT false,
    created_at timestamp with time zone DEFAULT now(),

    UNIQUE (challenge_id, user_id, day_number)
);

-- Index for fast lookups
CREATE INDEX IF NOT EXISTS idx_wake_up_challenges_creator ON public.wake_up_challenges(creator_id);
CREATE INDEX IF NOT EXISTS idx_wake_up_challenges_opponent ON public.wake_up_challenges(opponent_id);
CREATE INDEX IF NOT EXISTS idx_wake_up_entries_challenge ON public.wake_up_entries(challenge_id);

-- RLS policies
ALTER TABLE public.wake_up_challenges ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.wake_up_entries ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view their own challenges" ON public.wake_up_challenges
    FOR SELECT USING (creator_id = auth.uid() OR opponent_id = auth.uid());

CREATE POLICY "Users can create challenges" ON public.wake_up_challenges
    FOR INSERT WITH CHECK (creator_id = auth.uid());

CREATE POLICY "Users can update their own challenges" ON public.wake_up_challenges
    FOR UPDATE USING (creator_id = auth.uid() OR opponent_id = auth.uid());

CREATE POLICY "Users can view entries for their challenges" ON public.wake_up_entries
    FOR SELECT USING (
        challenge_id IN (
            SELECT id FROM public.wake_up_challenges
            WHERE creator_id = auth.uid() OR opponent_id = auth.uid()
        )
    );

CREATE POLICY "Users can insert their own entries" ON public.wake_up_entries
    FOR INSERT WITH CHECK (user_id = auth.uid());
