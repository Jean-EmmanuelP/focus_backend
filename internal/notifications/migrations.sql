-- =====================================================
-- NOTIFICATIONS TABLES
-- Run this SQL in Supabase SQL Editor
-- =====================================================

-- 1. Device Tokens Table
-- Stores FCM tokens for push notifications
CREATE TABLE IF NOT EXISTS device_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fcm_token TEXT NOT NULL,
    platform VARCHAR(20) DEFAULT 'ios', -- ios, android
    device_id TEXT, -- Optional device identifier
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Prevent duplicate tokens per user
    UNIQUE(user_id, fcm_token)
);

-- Index for quick lookups
CREATE INDEX IF NOT EXISTS idx_device_tokens_user_id ON device_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_device_tokens_active ON device_tokens(is_active) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_device_tokens_fcm ON device_tokens(fcm_token);

-- 2. Notification Preferences Table
-- Stores user notification settings
CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    morning_reminder_enabled BOOLEAN DEFAULT true,
    morning_reminder_time VARCHAR(5) DEFAULT '08:00', -- HH:MM format
    task_reminders_enabled BOOLEAN DEFAULT true,
    task_reminder_minutes_before INTEGER DEFAULT 15, -- 5, 10, 15, 30
    evening_reminder_enabled BOOLEAN DEFAULT false,
    evening_reminder_time VARCHAR(5) DEFAULT '21:00',
    streak_alert_enabled BOOLEAN DEFAULT true,
    weekly_summary_enabled BOOLEAN DEFAULT true,
    language VARCHAR(5) DEFAULT 'fr', -- fr, en
    timezone VARCHAR(50) DEFAULT 'Europe/Paris',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 3. Notification Events Table
-- Tracks all notifications sent for analytics
CREATE TABLE IF NOT EXISTS notification_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_id UUID NOT NULL UNIQUE, -- Unique ID sent with notification
    type VARCHAR(50) NOT NULL, -- morning, task_reminder, task_missed, evening, streak_danger, weekly_summary, quest_progress
    status VARCHAR(20) DEFAULT 'scheduled', -- scheduled, sent, delivered, opened, converted, failed
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    deep_link TEXT,
    metadata JSONB, -- Extra data (task_id, quest_id, etc.)
    scheduled_at TIMESTAMP WITH TIME ZONE,
    sent_at TIMESTAMP WITH TIME ZONE,
    opened_at TIMESTAMP WITH TIME ZONE,
    converted_at TIMESTAMP WITH TIME ZONE,
    action TEXT, -- What action user took after opening
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for analytics queries
CREATE INDEX IF NOT EXISTS idx_notification_events_user_id ON notification_events(user_id);
CREATE INDEX IF NOT EXISTS idx_notification_events_type ON notification_events(type);
CREATE INDEX IF NOT EXISTS idx_notification_events_status ON notification_events(status);
CREATE INDEX IF NOT EXISTS idx_notification_events_created ON notification_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_events_notification_id ON notification_events(notification_id);

-- 4. Motivational Phrases Table
-- Stores phrases for notifications (instead of hardcoding)
CREATE TABLE IF NOT EXISTS motivational_phrases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL, -- morning, task_reminder, task_missed, evening, streak_danger
    language VARCHAR(5) NOT NULL, -- fr, en
    phrase TEXT NOT NULL,
    is_active BOOLEAN DEFAULT true,
    usage_count INTEGER DEFAULT 0, -- Track how often used
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for random phrase selection
CREATE INDEX IF NOT EXISTS idx_motivational_phrases_type_lang ON motivational_phrases(type, language) WHERE is_active = true;

-- 5. Insert default French motivational phrases
INSERT INTO motivational_phrases (type, language, phrase) VALUES
-- Morning phrases (fr)
('morning', 'fr', 'C''est le moment de planifier ta journée et d''atteindre tes objectifs. Tu es capable de grandes choses !'),
('morning', 'fr', 'Chaque matin est une nouvelle opportunité. Qu''est-ce que tu vas accomplir aujourd''hui ?'),
('morning', 'fr', 'La réussite commence par un premier pas. Quel sera le tien aujourd''hui ?'),
('morning', 'fr', 'Aujourd''hui est le jour parfait pour avancer vers tes rêves. Go !'),
('morning', 'fr', 'Un nouveau jour, de nouvelles possibilités. Tu as le pouvoir de faire de cette journée quelque chose d''extraordinaire.'),
('morning', 'fr', 'Le succès n''est pas une destination, c''est un voyage. Profite de chaque étape !'),
('morning', 'fr', 'Ta motivation du jour : chaque petit progrès te rapproche de tes grands objectifs.'),
('morning', 'fr', 'Debout champion ! C''est l''heure de briller.'),
('morning', 'fr', 'Les grandes choses commencent par de petites actions quotidiennes. C''est parti !'),
('morning', 'fr', 'Aujourd''hui, tu as l''opportunité de devenir une meilleure version de toi-même.'),

-- Morning phrases (en)
('morning', 'en', 'Time to plan your day and crush your goals. You''ve got this!'),
('morning', 'en', 'Every morning is a fresh start. What will you accomplish today?'),
('morning', 'en', 'Success starts with a single step. What''s yours today?'),
('morning', 'en', 'Today is the perfect day to move closer to your dreams. Let''s go!'),
('morning', 'en', 'A new day, new possibilities. Make it count!'),
('morning', 'en', 'Small daily progress leads to big achievements. Start now!'),
('morning', 'en', 'Rise and shine! It''s your time to excel.'),
('morning', 'en', 'Great things start with small daily actions. Let''s begin!'),
('morning', 'en', 'Today, you have the chance to become a better version of yourself.'),
('morning', 'en', 'Your motivation for today: every small win brings you closer to your big goals.'),

-- Task reminder phrases (fr)
('task_reminder', 'fr', 'C''est l''heure de te concentrer ! Tu vas gérer.'),
('task_reminder', 'fr', 'Le moment est venu de passer à l''action. Tu peux le faire !'),
('task_reminder', 'fr', 'Focus mode: ON. Tu es prêt pour cette tâche !'),
('task_reminder', 'fr', 'C''est maintenant ! Montre ce dont tu es capable.'),
('task_reminder', 'fr', 'Rappel : tu as prévu cette tâche pour une bonne raison. Go !'),

-- Task reminder phrases (en)
('task_reminder', 'en', 'Time to focus! You''ve got this.'),
('task_reminder', 'en', 'It''s time to take action. You can do it!'),
('task_reminder', 'en', 'Focus mode: ON. You''re ready for this task!'),
('task_reminder', 'en', 'Now''s the time! Show what you''re made of.'),
('task_reminder', 'en', 'Reminder: you planned this task for a reason. Let''s go!'),

-- Task missed phrases (fr)
('task_missed', 'fr', 'Tu as manqué une tâche, mais ce n''est pas grave. Veux-tu la reprogrammer ?'),
('task_missed', 'fr', 'Une tâche t''attend encore. C''est le bon moment pour la terminer !'),
('task_missed', 'fr', 'Pas de stress ! Tu peux encore accomplir cette tâche aujourd''hui.'),

-- Task missed phrases (en)
('task_missed', 'en', 'You missed a task, but that''s okay. Want to reschedule it?'),
('task_missed', 'en', 'A task is still waiting for you. Now''s a good time to complete it!'),
('task_missed', 'en', 'No stress! You can still accomplish this task today.'),

-- Streak danger phrases (fr)
('streak_danger', 'fr', 'Attention ! Ta streak de %d jours est en danger. Complete une routine pour la maintenir !'),
('streak_danger', 'fr', 'Ne laisse pas ta streak s''arrêter ! Il te reste encore du temps.'),
('streak_danger', 'fr', 'Ta série est importante. Un petit effort maintenant pour la préserver !'),

-- Streak danger phrases (en)
('streak_danger', 'en', 'Warning! Your %d day streak is at risk. Complete a routine to keep it going!'),
('streak_danger', 'en', 'Don''t let your streak end! There''s still time.'),
('streak_danger', 'en', 'Your streak matters. A small effort now will preserve it!'),

-- Evening phrases (fr)
('evening', 'fr', 'Comment s''est passée ta journée ? Prends un moment pour faire le point.'),
('evening', 'fr', 'C''est l''heure du bilan ! Qu''as-tu accompli aujourd''hui ?'),
('evening', 'fr', 'Avant de te reposer, fais le point sur ta journée. Tu as fait du bon travail !'),

-- Evening phrases (en)
('evening', 'en', 'How was your day? Take a moment to reflect.'),
('evening', 'en', 'Time for your daily review! What did you accomplish today?'),
('evening', 'en', 'Before you rest, review your day. You did great work!')

ON CONFLICT DO NOTHING;

-- 6. Function to get random phrase
CREATE OR REPLACE FUNCTION get_random_phrase(p_type VARCHAR, p_language VARCHAR)
RETURNS TEXT AS $$
DECLARE
    result TEXT;
BEGIN
    SELECT phrase INTO result
    FROM motivational_phrases
    WHERE type = p_type
      AND language = p_language
      AND is_active = true
    ORDER BY RANDOM()
    LIMIT 1;

    -- Update usage count
    UPDATE motivational_phrases
    SET usage_count = usage_count + 1
    WHERE phrase = result AND type = p_type AND language = p_language;

    RETURN result;
END;
$$ LANGUAGE plpgsql;

-- 7. Enable Row Level Security
ALTER TABLE device_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_preferences ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_events ENABLE ROW LEVEL SECURITY;

-- RLS Policies (users can only access their own data)
CREATE POLICY "Users can manage own device tokens"
    ON device_tokens FOR ALL
    USING (user_id = auth.uid());

CREATE POLICY "Users can manage own notification preferences"
    ON notification_preferences FOR ALL
    USING (user_id = auth.uid());

CREATE POLICY "Users can view own notification events"
    ON notification_events FOR SELECT
    USING (user_id = auth.uid());

-- Service role can do everything (for backend cron jobs)
CREATE POLICY "Service role full access on device_tokens"
    ON device_tokens FOR ALL
    USING (auth.role() = 'service_role');

CREATE POLICY "Service role full access on notification_preferences"
    ON notification_preferences FOR ALL
    USING (auth.role() = 'service_role');

CREATE POLICY "Service role full access on notification_events"
    ON notification_events FOR ALL
    USING (auth.role() = 'service_role');

-- Grant access to service role
GRANT ALL ON device_tokens TO service_role;
GRANT ALL ON notification_preferences TO service_role;
GRANT ALL ON notification_events TO service_role;
GRANT ALL ON motivational_phrases TO service_role;
