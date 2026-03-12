-- Focus Rooms: group audio/video sessions by category
-- Users join rooms via matchmaking (backend finds or creates a room with < 6 participants)

-- 1. Rooms table
CREATE TABLE IF NOT EXISTS public.focus_rooms (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    category        text NOT NULL CHECK (category IN ('sport', 'travail', 'etudes', 'creativite', 'lecture', 'meditation')),
    livekit_room_name text NOT NULL UNIQUE,
    max_participants int NOT NULL DEFAULT 6,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    closed_at       timestamptz
);

-- 2. Room participants (join table)
CREATE TABLE IF NOT EXISTS public.focus_room_participants (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id     uuid NOT NULL REFERENCES public.focus_rooms(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    joined_at   timestamptz NOT NULL DEFAULT now(),
    left_at     timestamptz,
    UNIQUE(room_id, user_id)
);

-- 3. Indexes
CREATE INDEX IF NOT EXISTS idx_focus_rooms_category_status ON public.focus_rooms(category, status);
CREATE INDEX IF NOT EXISTS idx_focus_room_participants_room ON public.focus_room_participants(room_id);
CREATE INDEX IF NOT EXISTS idx_focus_room_participants_user ON public.focus_room_participants(user_id);

-- 4. RLS
ALTER TABLE public.focus_rooms ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.focus_room_participants ENABLE ROW LEVEL SECURITY;

-- Allow authenticated users to read rooms
CREATE POLICY "Users can read active rooms" ON public.focus_rooms
    FOR SELECT USING (status = 'active');

-- Allow backend (service role) to insert/update rooms
-- The backend uses the DB connection string directly (bypasses RLS via pgx),
-- so these policies are mainly for Supabase client access if needed.
CREATE POLICY "Service can manage rooms" ON public.focus_rooms
    FOR ALL USING (true) WITH CHECK (true);

CREATE POLICY "Users can read their participations" ON public.focus_room_participants
    FOR SELECT USING (user_id = auth.uid());

CREATE POLICY "Service can manage participations" ON public.focus_room_participants
    FOR ALL USING (true) WITH CHECK (true);
