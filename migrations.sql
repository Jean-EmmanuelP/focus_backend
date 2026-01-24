-- ⚠️ RESET: Drop everything to start fresh
drop table if exists public.focus_sessions cascade;
drop table if exists public.daily_reflections cascade;
drop table if exists public.routine_completions cascade;
drop table if exists public.routines cascade;
drop table if exists public.quests cascade;
drop table if exists public.areas cascade;

-- ==========================================
-- 1. AREAS (User Specific)
-- Each user defines their own buckets
-- ==========================================
create table public.areas (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  
  name text not null,          -- "Health", "Career"
  slug text,                   -- "health"
  -- color column removed
  icon text,                   -- "heart"
  
  completeness integer default 0, -- Stored % value (0-100)
  
  created_at timestamp with time zone default now()
);

-- RLS: Only own data
alter table public.areas enable row level security;
create policy "Users can manage own areas" on public.areas
  using (auth.uid() = user_id);


-- ==========================================
-- 2. QUESTS (Smart Tracking)
-- Tracks 4/12 books, etc.
-- ==========================================
create table public.quests (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  area_id uuid not null references public.areas on delete cascade,
  
  title text not null,         -- "Read 12 Books"
  status text default 'active', -- "active", "completed", "archived"
  
  current_value integer default 0, -- 4
  target_value integer default 1,  -- 12
  
  created_at timestamp with time zone default now()
);

-- RLS
alter table public.quests enable row level security;
create policy "Users can manage own quests" on public.quests
  using (auth.uid() = user_id);


-- ==========================================
-- 3. ROUTINES (Habits)
-- ==========================================
create table public.routines (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  area_id uuid references public.areas on delete cascade,

  title text not null,         -- "Drink 2L Water"
  frequency text default 'daily', -- "daily", "weekly"
  icon text,                   -- "water-drop"
  scheduled_time text,         -- HH:mm format for calendar display
  duration_minutes integer default 30, -- Duration for Google Calendar events

  created_at timestamp with time zone default now()
);

-- RLS
alter table public.routines enable row level security;
create policy "Users can manage own routines" on public.routines
  using (auth.uid() = user_id);


-- ==========================================
-- 4. ROUTINE COMPLETIONS (History Log)
-- Tracks every time a routine is done
-- ==========================================
create table public.routine_completions (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  routine_id uuid not null references public.routines on delete cascade,
  
  completed_at timestamp with time zone default now(),
  
  -- Constraint: Prevents double-logging the same routine on the same day
  unique (user_id, routine_id, completed_at) 
);

-- RLS
alter table public.routine_completions enable row level security;
create policy "Users can manage own completions" on public.routine_completions
  using (auth.uid() = user_id);

-- ==========================================
-- 5. DAILY REFLECTIONS (Journaling)
-- ==========================================
create table public.daily_reflections (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  
  -- The specific date this entry is for (e.g., '2023-10-27')
  date date default current_date not null,
  
  -- The 4 Input Fields
  biggest_win text,           -- "What made you proud today?"
  challenges text,            -- "What blocked or stressed you?"
  best_moment text,           -- "A moment that made you smile..."
  goal_for_tomorrow text,     -- "What will you focus on tomorrow?"
  
  created_at timestamp with time zone default now(),
  
  -- Constraint: Only one reflection per user per day.
  unique (user_id, date)
);

-- RLS: Only own data
alter table public.daily_reflections enable row level security;

create policy "Users can manage own reflections" 
on public.daily_reflections
using (auth.uid() = user_id);

-- ==========================================
-- 6. FOCUS SESSIONS (Pomodoro / Deep Work)
-- ==========================================
create table public.focus_sessions (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  
  -- Optional link to a specific Quest
  -- "on delete set null" means if the Quest is deleted, we keep the session history, just unlink it.
  quest_id uuid references public.quests on delete set null,
  
  description text,              -- "What will you work on?"
  duration_minutes integer not null, -- 25, 50, 90, etc.
  
  -- Status tracking
  status text default 'active',  -- 'active', 'completed', 'cancelled'
  
  -- Timestamps
  started_at timestamp with time zone default now(),
  completed_at timestamp with time zone, -- Null until finished
  
  created_at timestamp with time zone default now()
);

-- RLS
alter table public.focus_sessions enable row level security;

create policy "Users can manage own sessions"
on public.focus_sessions
using (auth.uid() = user_id);


-- ==========================================
-- 7. TASKS (Calendar Tasks - unified with time_blocks)
-- ==========================================
create table public.tasks (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  -- Optional links
  quest_id uuid references public.quests on delete set null,
  area_id uuid references public.areas on delete set null,

  -- Core fields
  title text not null,
  description text,
  position integer default 0,

  -- Time estimates
  estimated_minutes integer,
  actual_minutes integer default 0,

  -- Priority and status
  priority text default 'medium',  -- low, medium, high, urgent
  status text default 'pending',   -- pending, in_progress, completed

  -- Calendar scheduling
  date date default current_date,
  scheduled_start time,            -- HH:mm format in DB
  scheduled_end time,              -- HH:mm format in DB
  time_block text default 'morning', -- morning, afternoon, evening

  -- Due date and completion
  due_at timestamp with time zone,
  completed_at timestamp with time zone,

  -- AI fields
  is_ai_generated boolean default false,
  ai_notes text,

  -- Google Calendar sync fields
  google_event_id text,
  google_calendar_id text,
  last_synced_at timestamp with time zone,

  -- Timestamps
  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now()
);

-- RLS
alter table public.tasks enable row level security;
create policy "Users can manage own tasks" on public.tasks
  using (auth.uid() = user_id);

-- Indexes for performance
create index idx_tasks_user_date on public.tasks(user_id, date);
create index idx_tasks_quest on public.tasks(quest_id);
create index idx_tasks_area on public.tasks(area_id);


-- ==========================================
-- 8. DAY PLANS (For AI day planning)
-- ==========================================
create table public.day_plans (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  date date not null,
  ideal_day_prompt text,
  ai_summary text,
  progress integer default 0,
  status text default 'active',

  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now(),

  unique (user_id, date)
);

-- RLS
alter table public.day_plans enable row level security;
create policy "Users can manage own day_plans" on public.day_plans
  using (auth.uid() = user_id);


-- ==========================================
-- 9. FRIEND GROUPS (Custom friend groups)
-- ==========================================
create table public.friend_groups (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  name text not null,            -- "Gym Buddies", "Work Team"
  description text,              -- Optional description
  icon text default '👥',        -- Emoji icon
  color text default '#6366F1',  -- Hex color for display

  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now()
);

-- RLS: Only own groups
alter table public.friend_groups enable row level security;
create policy "Users can manage own friend_groups" on public.friend_groups
  using (auth.uid() = user_id);

-- Indexes
create index idx_friend_groups_user on public.friend_groups(user_id);


-- ==========================================
-- 10. FRIEND GROUP MEMBERS (Members in groups)
-- ==========================================
create table public.friend_group_members (
  id uuid default gen_random_uuid() primary key,
  group_id uuid not null references public.friend_groups on delete cascade,
  member_id uuid not null references auth.users on delete cascade,

  added_at timestamp with time zone default now(),

  -- Prevent duplicate members in same group
  unique (group_id, member_id)
);

-- RLS: Users can manage members of their own groups
alter table public.friend_group_members enable row level security;
create policy "Users can manage own friend_group_members" on public.friend_group_members
  using (
    exists (
      select 1 from public.friend_groups g
      where g.id = group_id and g.user_id = auth.uid()
    )
  );

-- Indexes
create index idx_friend_group_members_group on public.friend_group_members(group_id);
create index idx_friend_group_members_member on public.friend_group_members(member_id);


-- ==========================================
-- 10b. GROUP ROUTINES (Shared Routines)
-- ==========================================
-- Links routines to groups for shared accountability
create table public.group_routines (
  id uuid default gen_random_uuid() primary key,
  group_id uuid not null references public.friend_groups on delete cascade,
  routine_id uuid not null references public.routines on delete cascade,
  shared_by uuid not null references auth.users on delete cascade,
  created_at timestamp with time zone default now(),

  -- A routine can only be shared once per group
  unique (group_id, routine_id)
);

-- RLS: Group members can read, sharer/owner can manage
alter table public.group_routines enable row level security;

-- Group members can view shared routines
create policy "Group members can view shared routines" on public.group_routines
  for select using (
    exists (
      select 1 from public.friend_groups g
      where g.id = group_id and g.user_id = auth.uid()
    )
    or exists (
      select 1 from public.friend_group_members gm
      where gm.group_id = group_routines.group_id and gm.member_id = auth.uid()
    )
  );

-- Users can share their own routines
create policy "Users can share own routines" on public.group_routines
  for insert with check (
    auth.uid() = shared_by
    and exists (
      select 1 from public.routines r
      where r.id = routine_id and r.user_id = auth.uid()
    )
  );

-- Group owner or sharer can delete
create policy "Owner or sharer can delete" on public.group_routines
  for delete using (
    auth.uid() = shared_by
    or exists (
      select 1 from public.friend_groups g
      where g.id = group_id and g.user_id = auth.uid()
    )
  );

-- Indexes
create index idx_group_routines_group on public.group_routines(group_id);
create index idx_group_routines_routine on public.group_routines(routine_id);


-- ==========================================
-- 11. COMMUNITY POSTS (Social Feed)
-- ==========================================
create table public.community_posts (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  -- Link to task OR routine (at least one required)
  task_id uuid references public.tasks on delete set null,
  routine_id uuid references public.routines on delete set null,

  -- Content
  image_url text not null,           -- URL to Supabase Storage
  caption text,                      -- Optional description

  -- Engagement
  likes_count integer default 0,

  -- Moderation
  is_hidden boolean default false,   -- Hidden if reported & reviewed

  created_at timestamp with time zone default now()
  -- task_id and routine_id are optional
);

-- RLS: Public read (non-hidden), own write
alter table public.community_posts enable row level security;

-- Anyone can read visible posts
create policy "Anyone can read visible posts" on public.community_posts
  for select using (is_hidden = false);

-- Users can manage own posts
create policy "Users can manage own posts" on public.community_posts
  for all using (auth.uid() = user_id);

-- Indexes
create index idx_community_posts_user on public.community_posts(user_id);
create index idx_community_posts_created on public.community_posts(created_at desc);
create index idx_community_posts_task on public.community_posts(task_id) where task_id is not null;
create index idx_community_posts_routine on public.community_posts(routine_id) where routine_id is not null;
create index idx_community_posts_visible on public.community_posts(created_at desc) where is_hidden = false;


-- ==========================================
-- 12. COMMUNITY POST LIKES
-- ==========================================
create table public.community_post_likes (
  id uuid default gen_random_uuid() primary key,
  post_id uuid not null references public.community_posts on delete cascade,
  user_id uuid not null references auth.users on delete cascade,

  created_at timestamp with time zone default now(),

  -- Prevent duplicate likes
  unique (post_id, user_id)
);

-- RLS
alter table public.community_post_likes enable row level security;

create policy "Anyone can read likes" on public.community_post_likes
  for select using (true);

create policy "Users can manage own likes" on public.community_post_likes
  for all using (auth.uid() = user_id);

-- Indexes
create index idx_community_post_likes_post on public.community_post_likes(post_id);
create index idx_community_post_likes_user on public.community_post_likes(user_id);


-- ==========================================
-- 13. COMMUNITY POST REPORTS (Moderation)
-- ==========================================
create table public.community_post_reports (
  id uuid default gen_random_uuid() primary key,
  post_id uuid not null references public.community_posts on delete cascade,
  reporter_id uuid not null references auth.users on delete cascade,

  reason text not null,              -- 'inappropriate', 'spam', 'harassment', 'other'
  details text,                      -- Optional additional details
  status text default 'pending',     -- 'pending', 'reviewed', 'dismissed'

  created_at timestamp with time zone default now(),

  -- One report per user per post
  unique (post_id, reporter_id)
);

-- RLS: Only admins should review reports, but users can create their own
alter table public.community_post_reports enable row level security;

create policy "Users can create own reports" on public.community_post_reports
  for insert with check (auth.uid() = reporter_id);

create policy "Users can view own reports" on public.community_post_reports
  for select using (auth.uid() = reporter_id);

-- Index for pending reports
create index idx_community_post_reports_pending on public.community_post_reports(created_at) where status = 'pending';


-- ==========================================
-- 14. JOURNAL ENTRIES (Audio/Video Progress Journal)
-- ==========================================
create table public.journal_entries (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  -- Media
  media_type text not null check (media_type in ('audio', 'video')),
  media_url text not null,
  duration_seconds integer not null,

  -- AI Analysis
  transcript text,
  summary text,              -- 3-5 bullet points
  title text,                -- AI-generated title
  mood text,                 -- 'great', 'good', 'neutral', 'low', 'bad'
  mood_score integer,        -- 1-10 for graphing
  tags text[],               -- AI-detected themes

  -- Metadata
  entry_date date not null default current_date,
  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now(),

  -- One entry per day per user
  constraint unique_daily_entry unique (user_id, entry_date)
);

-- RLS
alter table public.journal_entries enable row level security;

create policy "Users can manage own journal entries" on public.journal_entries
  using (auth.uid() = user_id);

-- Indexes
create index idx_journal_entries_user on public.journal_entries(user_id);
create index idx_journal_entries_date on public.journal_entries(entry_date desc);
create index idx_journal_entries_user_date on public.journal_entries(user_id, entry_date desc);


-- ==========================================
-- 15. JOURNAL BILANS (Weekly/Monthly Summaries)
-- ==========================================
create table public.journal_bilans (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  bilan_type text not null check (bilan_type in ('weekly', 'monthly')),
  period_start date not null,
  period_end date not null,

  -- AI-generated content
  summary text not null,
  wins text[],
  improvements text[],
  mood_trend text,             -- 'improving', 'stable', 'declining'
  avg_mood_score decimal(3,1),
  suggested_goals text[],      -- For monthly only

  created_at timestamp with time zone default now(),

  -- One bilan per type per period per user
  constraint unique_bilan unique (user_id, bilan_type, period_start)
);

-- RLS
alter table public.journal_bilans enable row level security;

create policy "Users can manage own journal bilans" on public.journal_bilans
  using (auth.uid() = user_id);

-- Indexes
create index idx_journal_bilans_user on public.journal_bilans(user_id);
create index idx_journal_bilans_period on public.journal_bilans(user_id, bilan_type, period_start desc);


-- ==========================================
-- 16. GOOGLE CALENDAR CONFIG
-- ==========================================
create table public.google_calendar_config (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  -- OAuth tokens
  access_token text not null,
  refresh_token text not null,
  token_expiry timestamp with time zone not null,

  -- Configuration
  is_enabled boolean default true,
  sync_direction text default 'bidirectional', -- bidirectional, to_google, from_google
  calendar_id text default 'primary',
  google_email text,
  timezone text default 'Europe/Paris', -- User's timezone for Google Calendar events

  -- Sync tracking
  last_sync_at timestamp with time zone,
  last_routine_sync_at timestamp with time zone,

  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now(),

  -- One config per user
  unique (user_id)
);

-- RLS
alter table public.google_calendar_config enable row level security;
create policy "Users can manage own google_calendar_config" on public.google_calendar_config
  using (auth.uid() = user_id);


-- ==========================================
-- 17. ROUTINE GOOGLE EVENTS
-- Tracks weekly routine events in Google Calendar
-- ==========================================
create table public.routine_google_events (
  routine_id uuid not null references public.routines on delete cascade,
  user_id uuid not null references auth.users on delete cascade,
  google_event_id text not null,
  google_calendar_id text not null,
  event_date date not null,

  created_at timestamp with time zone default now(),

  -- One event per routine per date
  primary key (routine_id, event_date)
);

-- RLS
alter table public.routine_google_events enable row level security;
create policy "Users can manage own routine_google_events" on public.routine_google_events
  using (auth.uid() = user_id);

-- Indexes
create index idx_routine_google_events_user on public.routine_google_events(user_id);
create index idx_routine_google_events_routine on public.routine_google_events(routine_id);

-- ==========================================
-- 18. USER PRODUCTIVITY PREFERENCES
-- Add productivity_peak column to users
-- Values: 'morning', 'afternoon', 'evening'
-- ==========================================
alter table public.users add column if not exists productivity_peak text;


-- ==========================================
-- 19. WEEKLY GOALS
-- User's weekly intentions/objectives
-- Exactly like daily intentions but for the week
-- ==========================================
create table public.weekly_goals (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  week_start_date date not null,      -- Monday of the week

  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now(),

  -- One weekly goal set per user per week
  unique (user_id, week_start_date)
);

-- RLS
alter table public.weekly_goals enable row level security;
create policy "Users can manage own weekly_goals" on public.weekly_goals
  using (auth.uid() = user_id);

-- Indexes
create index idx_weekly_goals_user on public.weekly_goals(user_id);
create index idx_weekly_goals_week on public.weekly_goals(user_id, week_start_date desc);


-- ==========================================
-- 20. WEEKLY GOAL ITEMS
-- Individual goals within a week (like intention_items)
-- ==========================================
create table public.weekly_goal_items (
  id uuid default gen_random_uuid() primary key,
  weekly_goal_id uuid not null references public.weekly_goals on delete cascade,

  area_id uuid references public.areas on delete set null,  -- Optional link to life area (for emoji)
  content text not null,                                      -- Goal text
  position integer default 0,                                 -- Order

  is_completed boolean default false,
  completed_at timestamp with time zone,

  created_at timestamp with time zone default now()
);

-- RLS
alter table public.weekly_goal_items enable row level security;
create policy "Users can manage own weekly_goal_items" on public.weekly_goal_items
  using (
    weekly_goal_id in (
      select id from public.weekly_goals where user_id = auth.uid()
    )
  );

-- Indexes
create index idx_weekly_goal_items_goal on public.weekly_goal_items(weekly_goal_id);
create index idx_weekly_goal_items_position on public.weekly_goal_items(weekly_goal_id, position);


-- ==========================================
-- 21. CHAT MESSAGES (AI Coach Conversations)
-- Persistent conversation history with Kai coach
-- ==========================================
create table public.chat_messages (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  -- Message content
  content text not null,
  is_from_user boolean default true,
  message_type text default 'text',  -- 'text', 'voice', 'toolCard', 'dailyStats'

  -- Voice message data (optional)
  voice_url text,                    -- URL to audio file in storage
  voice_transcript text,             -- Transcription of voice message

  -- Tool action (optional)
  tool_action text,                  -- 'planDay', 'weeklyGoals', 'dailyReflection', 'startFocus', 'viewStats', 'logMood'

  -- Timestamps
  created_at timestamp with time zone default now()
);

-- RLS
alter table public.chat_messages enable row level security;
create policy "Users can manage own chat_messages" on public.chat_messages
  using (auth.uid() = user_id);

-- Indexes
create index idx_chat_messages_user on public.chat_messages(user_id);
create index idx_chat_messages_created on public.chat_messages(user_id, created_at desc);


-- ==========================================
-- 22. CHAT CONTEXT (Semantic Memory Storage)
-- Stores facts with vector embeddings for semantic search
-- Inspired by Mira architecture + pgvector
-- ==========================================

-- Enable pgvector extension (run in Supabase dashboard first)
-- create extension if not exists vector;

create table public.chat_contexts (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,

  -- Semantic memory fields
  fact text not null,                -- The fact/memory extracted
  category text not null default 'personal', -- personal, work, goals, preferences, emotions, relationship
  mention_count integer not null default 1,  -- How many times this was mentioned
  first_mentioned timestamp with time zone default now(),
  last_mentioned timestamp with time zone default now(),

  -- Mira-style fields
  confidence float default 0.8,      -- Confidence score 0-1
  entities text[] default '{}',      -- Named entities (people, places, etc.)

  -- Vector embedding (768 dimensions for text-embedding-004)
  embedding vector(768),

  -- Timestamps
  created_at timestamp with time zone default now(),

  -- Same fact from same user should be unique (for exact text dedup)
  unique (user_id, fact)
);

-- RLS
alter table public.chat_contexts enable row level security;
create policy "Users can manage own chat_contexts" on public.chat_contexts
  using (auth.uid() = user_id);

-- Indexes
create index idx_chat_contexts_user on public.chat_contexts(user_id);
create index idx_chat_contexts_last_mentioned on public.chat_contexts(user_id, last_mentioned desc);
create index idx_chat_contexts_category on public.chat_contexts(user_id, category);

-- HNSW index for fast vector similarity search
create index idx_chat_contexts_embedding on public.chat_contexts
  using hnsw (embedding vector_cosine_ops);

-- Function: Semantic similarity search
create or replace function match_memories(
  query_embedding vector(768),
  match_user_id uuid,
  match_threshold float default 0.5,
  match_count int default 10
)
returns table (
  id uuid,
  fact text,
  category text,
  mention_count int,
  first_mentioned timestamptz,
  last_mentioned timestamptz,
  similarity float
)
language plpgsql
as $$
begin
  return query
  select
    c.id,
    c.fact,
    c.category,
    c.mention_count,
    c.first_mentioned,
    c.last_mentioned,
    1 - (c.embedding <=> query_embedding) as similarity
  from chat_contexts c
  where c.user_id = match_user_id
    and c.embedding is not null
    and 1 - (c.embedding <=> query_embedding) > match_threshold
  order by c.embedding <=> query_embedding
  limit match_count;
end;
$$;

-- Function: Find similar memory for deduplication (85% threshold)
create or replace function find_similar_memory(
  query_embedding vector(768),
  match_user_id uuid,
  similarity_threshold float default 0.85
)
returns table (
  id uuid,
  fact text,
  similarity float
)
language plpgsql
as $$
begin
  return query
  select
    c.id,
    c.fact,
    1 - (c.embedding <=> query_embedding) as similarity
  from chat_contexts c
  where c.user_id = match_user_id
    and c.embedding is not null
    and 1 - (c.embedding <=> query_embedding) >= similarity_threshold
  order by c.embedding <=> query_embedding
  limit 1;
end;
$$;


-- ==========================================
-- 23. WHATSAPP USERS (Linked WhatsApp Accounts)
-- ==========================================
create table public.whatsapp_users (
  id uuid default gen_random_uuid() primary key,
  user_id uuid references auth.users on delete cascade unique,  -- Optional: linked app user
  phone_number text not null unique,
  display_name text,

  -- Linking status
  is_linked boolean default false,  -- True if linked to app account
  linked_at timestamp with time zone,

  -- WhatsApp-only onboarding
  onboarding_step text default 'welcome',  -- welcome, ready

  -- Preferences (stored as JSONB)
  preferences jsonb default '{
    "morning_check_in": true,
    "morning_check_in_time": "08:00",
    "evening_review": true,
    "evening_review_time": "21:00",
    "streak_alerts": true,
    "quest_reminders": true,
    "inactivity_reminders": true
  }'::jsonb,

  -- Timestamps
  created_at timestamp with time zone default now(),
  updated_at timestamp with time zone default now()
);

-- RLS: Users can only see their own WhatsApp link
alter table public.whatsapp_users enable row level security;
create policy "Users can manage own whatsapp_users" on public.whatsapp_users
  using (auth.uid() = user_id);

-- Indexes
create index idx_whatsapp_users_phone on public.whatsapp_users(phone_number);
create index idx_whatsapp_users_user on public.whatsapp_users(user_id);


-- ==========================================
-- 24. WHATSAPP VERIFICATION CODES
-- ==========================================
create table public.whatsapp_verification_codes (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade unique,
  phone_number text not null,
  code text not null,
  expires_at timestamp with time zone not null,
  created_at timestamp with time zone default now()
);

-- RLS
alter table public.whatsapp_verification_codes enable row level security;
create policy "Users can manage own verification_codes" on public.whatsapp_verification_codes
  using (auth.uid() = user_id);


-- ==========================================
-- 25. WHATSAPP OTP (Email Verification for Linking)
-- Used when linking from WhatsApp to existing app account
-- ==========================================
create table public.whatsapp_otp (
  id uuid default gen_random_uuid() primary key,
  user_id uuid not null references auth.users on delete cascade,
  phone_number text not null,
  otp text not null,
  expires_at timestamp with time zone not null,
  verified boolean default false,
  created_at timestamp with time zone default now()
);

-- Indexes
create index idx_whatsapp_otp_phone on public.whatsapp_otp(phone_number);
create index idx_whatsapp_otp_user on public.whatsapp_otp(user_id);

-- ==========================================
-- ADD BLOCK_APPS TO TASKS
-- When true, iOS will block distracting apps during this task
-- ==========================================
ALTER TABLE public.tasks ADD COLUMN IF NOT EXISTS block_apps boolean default false;
ALTER TABLE public.tasks ADD COLUMN IF NOT EXISTS is_private boolean default false;