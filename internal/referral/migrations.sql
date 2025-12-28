-- =====================================================
-- REFERRAL / PARRAINAGE TABLES
-- Run this SQL in Supabase SQL Editor
-- =====================================================

-- 1. Referral Codes Table
-- Each user gets a unique referral code
CREATE TABLE IF NOT EXISTS referral_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    code VARCHAR(12) NOT NULL UNIQUE, -- e.g., "JEAN2024" or "FOCUS-ABC123"
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_referral_codes_user_id ON referral_codes(user_id);
CREATE INDEX IF NOT EXISTS idx_referral_codes_code ON referral_codes(code);

-- 2. Referrals Table
-- Tracks who invited whom
CREATE TABLE IF NOT EXISTS referrals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    referrer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- The person who invited
    referred_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE, -- The person who signed up
    referral_code_id UUID NOT NULL REFERENCES referral_codes(id) ON DELETE CASCADE,
    status VARCHAR(20) DEFAULT 'pending', -- pending, active, churned
    referred_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(), -- When they signed up
    activated_at TIMESTAMP WITH TIME ZONE, -- When they became a paying subscriber
    churned_at TIMESTAMP WITH TIME ZONE, -- When they cancelled
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_referrals_referrer_id ON referrals(referrer_id);
CREATE INDEX IF NOT EXISTS idx_referrals_referred_id ON referrals(referred_id);
CREATE INDEX IF NOT EXISTS idx_referrals_status ON referrals(status);

-- 3. Referral Earnings Table
-- Tracks monthly commissions earned
CREATE TABLE IF NOT EXISTS referral_earnings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    referrer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    referral_id UUID NOT NULL REFERENCES referrals(id) ON DELETE CASCADE,
    month DATE NOT NULL, -- First day of the month (e.g., 2024-01-01)
    subscription_amount DECIMAL(10,2) NOT NULL, -- What the referred user paid
    commission_rate DECIMAL(5,4) DEFAULT 0.20, -- 20% = 0.20
    commission_amount DECIMAL(10,2) NOT NULL, -- subscription_amount * commission_rate
    status VARCHAR(20) DEFAULT 'pending', -- pending, credited, paid_out
    credited_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Prevent duplicate earnings for same referral in same month
    UNIQUE(referral_id, month)
);

CREATE INDEX IF NOT EXISTS idx_referral_earnings_referrer_id ON referral_earnings(referrer_id);
CREATE INDEX IF NOT EXISTS idx_referral_earnings_month ON referral_earnings(month);
CREATE INDEX IF NOT EXISTS idx_referral_earnings_status ON referral_earnings(status);

-- 4. Referral Credits Table
-- User's accumulated credit balance (for free months)
CREATE TABLE IF NOT EXISTS referral_credits (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    total_earned DECIMAL(10,2) DEFAULT 0, -- Total ever earned
    total_used DECIMAL(10,2) DEFAULT 0, -- Total ever used
    current_balance DECIMAL(10,2) DEFAULT 0, -- total_earned - total_used
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 5. Referral Stats View (for quick lookups)
CREATE OR REPLACE VIEW referral_stats AS
SELECT
    rc.user_id,
    rc.code,
    COUNT(DISTINCT r.id) FILTER (WHERE r.status IN ('pending', 'active')) as total_referrals,
    COUNT(DISTINCT r.id) FILTER (WHERE r.status = 'active') as active_referrals,
    COALESCE(SUM(re.commission_amount) FILTER (WHERE re.status = 'credited'), 0) as total_earned,
    COALESCE(cred.current_balance, 0) as current_balance
FROM referral_codes rc
LEFT JOIN referrals r ON r.referral_code_id = rc.id
LEFT JOIN referral_earnings re ON re.referrer_id = rc.user_id
LEFT JOIN referral_credits cred ON cred.user_id = rc.user_id
GROUP BY rc.user_id, rc.code, cred.current_balance;

-- 6. Function to generate unique referral code
CREATE OR REPLACE FUNCTION generate_referral_code(p_user_id UUID)
RETURNS VARCHAR AS $$
DECLARE
    v_code VARCHAR(12);
    v_exists BOOLEAN;
    v_first_name VARCHAR;
    v_attempts INT := 0;
BEGIN
    -- Get user's first name
    SELECT UPPER(LEFT(COALESCE(first_name, pseudo, 'FOCUS'), 4)) INTO v_first_name
    FROM users WHERE id = p_user_id;

    LOOP
        -- Generate code: NAME + random alphanumeric
        v_code := v_first_name || '-' || UPPER(SUBSTRING(MD5(RANDOM()::TEXT) FROM 1 FOR 5));

        -- Check if exists
        SELECT EXISTS(SELECT 1 FROM referral_codes WHERE code = v_code) INTO v_exists;

        EXIT WHEN NOT v_exists OR v_attempts > 10;
        v_attempts := v_attempts + 1;
    END LOOP;

    RETURN v_code;
END;
$$ LANGUAGE plpgsql;

-- 7. Function to get or create referral code for user
CREATE OR REPLACE FUNCTION get_or_create_referral_code(p_user_id UUID)
RETURNS VARCHAR AS $$
DECLARE
    v_code VARCHAR(12);
BEGIN
    -- Try to get existing code
    SELECT code INTO v_code FROM referral_codes WHERE user_id = p_user_id;

    IF v_code IS NULL THEN
        -- Generate new code
        v_code := generate_referral_code(p_user_id);

        -- Insert it
        INSERT INTO referral_codes (user_id, code)
        VALUES (p_user_id, v_code);
    END IF;

    RETURN v_code;
END;
$$ LANGUAGE plpgsql;

-- 8. Enable Row Level Security
ALTER TABLE referral_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE referrals ENABLE ROW LEVEL SECURITY;
ALTER TABLE referral_earnings ENABLE ROW LEVEL SECURITY;
ALTER TABLE referral_credits ENABLE ROW LEVEL SECURITY;

-- RLS Policies
CREATE POLICY "Users can view own referral code" ON referral_codes
    FOR SELECT USING (user_id = auth.uid());

CREATE POLICY "Users can view referrals they made" ON referrals
    FOR SELECT USING (referrer_id = auth.uid());

CREATE POLICY "Users can view own earnings" ON referral_earnings
    FOR SELECT USING (referrer_id = auth.uid());

CREATE POLICY "Users can view own credits" ON referral_credits
    FOR SELECT USING (user_id = auth.uid());

-- Service role full access (for backend)
CREATE POLICY "Service role full access referral_codes" ON referral_codes
    FOR ALL USING (auth.role() = 'service_role');

CREATE POLICY "Service role full access referrals" ON referrals
    FOR ALL USING (auth.role() = 'service_role');

CREATE POLICY "Service role full access referral_earnings" ON referral_earnings
    FOR ALL USING (auth.role() = 'service_role');

CREATE POLICY "Service role full access referral_credits" ON referral_credits
    FOR ALL USING (auth.role() = 'service_role');

-- Grant access
GRANT ALL ON referral_codes TO service_role;
GRANT ALL ON referrals TO service_role;
GRANT ALL ON referral_earnings TO service_role;
GRANT ALL ON referral_credits TO service_role;
GRANT SELECT ON referral_stats TO service_role;
