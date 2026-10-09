ALTER TYPE merchant_lifecycle_action ADD VALUE IF NOT EXISTS 'ACTIVATE';
ALTER TYPE merchant_lifecycle_action ADD VALUE IF NOT EXISTS 'SUSPEND';
ALTER TYPE merchant_lifecycle_action ADD VALUE IF NOT EXISTS 'REACTIVATE';
ALTER TYPE merchant_lifecycle_action ADD VALUE IF NOT EXISTS 'APPROVE';
ALTER TYPE merchant_lifecycle_action ADD VALUE IF NOT EXISTS 'REJECT';
ALTER TYPE merchant_lifecycle_action ADD VALUE IF NOT EXISTS 'UPDATE_STATUS';
