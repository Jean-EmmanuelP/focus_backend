-- ==========================================
-- MIGRATIONS V2 - New features for Focus PRD
-- Run after migrations.sql
-- ==========================================

-- ==========================================
-- 1. DEVICE TOKENS (Push Notifications - APNs)
-- ==========================================
CREATE TABLE IF NOT EXISTS public.device_tokens (
  id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auth.users ON DELETE CASCADE,
  token text NOT NULL,
  platform text NOT NULL DEFAULT 'ios',       -- ios, ipados
  app_version text,
  created_at timestamp with time zone DEFAULT now(),
  updated_at timestamp with time zone DEFAULT now(),

  -- One token per user per platform
  UNIQUE (user_id, platform)
);

ALTER TABLE public.device_tokens ENABLE ROW LEVEL SECURITY;
CREATE POLICY "Users can manage own device_tokens" ON public.device_tokens
  USING (auth.uid() = user_id);

CREATE INDEX IF NOT EXISTS idx_device_tokens_user ON public.device_tokens(user_id);

-- ==========================================
-- 2. NOTIFICATION SETTINGS (JSONB on users)
-- ==========================================
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS notification_settings jsonb DEFAULT '{
  "focus_reminders": true,
  "ritual_reminders": true,
  "morning_checkin": true,
  "evening_checkin": true,
  "streak_alerts": true,
  "quest_milestones": true,
  "crew_activity": true,
  "leaderboard_updates": false
}'::jsonb;

-- ==========================================
-- 3. MORNING CHECK-INS
-- ==========================================
CREATE TABLE IF NOT EXISTS public.morning_checkins (
  id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auth.users ON DELETE CASCADE,

  date date NOT NULL DEFAULT CURRENT_DATE,
  morning_mood integer NOT NULL CHECK (morning_mood BETWEEN 1 AND 5),
  sleep_quality integer NOT NULL CHECK (sleep_quality BETWEEN 1 AND 5),
  intentions jsonb DEFAULT '[]'::jsonb,        -- Array of intention strings
  top_priority text,
  energy_level integer CHECK (energy_level BETWEEN 1 AND 5),

  created_at timestamp with time zone DEFAULT now(),
  updated_at timestamp with time zone DEFAULT now(),

  UNIQUE (user_id, date)
);

ALTER TABLE public.morning_checkins ENABLE ROW LEVEL SECURITY;
CREATE POLICY "Users can manage own morning_checkins" ON public.morning_checkins
  USING (auth.uid() = user_id);

CREATE INDEX IF NOT EXISTS idx_morning_checkins_user_date ON public.morning_checkins(user_id, date DESC);

-- ==========================================
-- 4. EVENING CHECK-INS
-- ==========================================
CREATE TABLE IF NOT EXISTS public.evening_checkins (
  id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auth.users ON DELETE CASCADE,

  date date NOT NULL DEFAULT CURRENT_DATE,
  evening_mood integer NOT NULL CHECK (evening_mood BETWEEN 1 AND 5),
  biggest_win text,
  blockers text,
  rituals_completed integer DEFAULT 0,
  tasks_completed integer DEFAULT 0,
  focus_minutes integer DEFAULT 0,
  goal_for_tomorrow text,
  grateful_for text,

  created_at timestamp with time zone DEFAULT now(),
  updated_at timestamp with time zone DEFAULT now(),

  UNIQUE (user_id, date)
);

ALTER TABLE public.evening_checkins ENABLE ROW LEVEL SECURITY;
CREATE POLICY "Users can manage own evening_checkins" ON public.evening_checkins
  USING (auth.uid() = user_id);

CREATE INDEX IF NOT EXISTS idx_evening_checkins_user_date ON public.evening_checkins(user_id, date DESC);

-- ==========================================
-- 5. ONBOARDING - Add responses JSONB column
-- ==========================================
ALTER TABLE public.user_onboarding ADD COLUMN IF NOT EXISTS responses jsonb DEFAULT '{}'::jsonb;
