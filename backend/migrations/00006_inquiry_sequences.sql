-- +goose Up
-- Allocate enquiry reference numbers from sequences instead of MAX()+1.
--
-- The retry loop it replaces could not work under real concurrency, and the
-- reason is not the retry count. MAX(reference_no)+1 reads only COMMITTED rows,
-- so N concurrent submissions all read the same maximum and then walk the same
-- candidates in lockstep: attempt 0 collides for all but one, attempt 1 collides
-- for all but one, and so on. The collisions are systematic rather than random,
-- so the loop needs N attempts to serve N visitors and any bound below that
-- turns into "не удалось" on a public form. At 12 concurrent it failed outright.
--
-- A sequence is contention-free by construction and never hands the same value
-- to two callers. It leaves gaps when a transaction rolls back — a reference
-- number is an identifier the visitor quotes back, not a count of anything, so a
-- gap costs nothing.
--
-- One sequence per type, because docs/05-MODULES.md:160 numbers each type
-- independently: WR-0001 and CF-0001 both exist and are different enquiries.

-- +goose StatementBegin
DO $$
DECLARE
  p text;
BEGIN
  FOREACH p IN ARRAY ARRAY['wr','cf','da','cp','jb'] LOOP
    EXECUTE format('CREATE SEQUENCE IF NOT EXISTS inquiry_ref_%s START 1', p);
  END LOOP;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION next_inquiry_reference(p_prefix text)
RETURNS text AS $$
DECLARE
  key text;
  n   bigint;
BEGIN
  -- Validated against a fixed set rather than interpolated directly. The caller
  -- is a Go constant map today, but this function builds a sequence name by
  -- string formatting, and a name that can be influenced from outside is an
  -- injection surface regardless of who happens to call it now.
  key := lower(rtrim(p_prefix, '-'));
  IF key NOT IN ('wr','cf','da','cp','jb') THEN
    RAISE EXCEPTION 'unknown inquiry prefix: %', p_prefix;
  END IF;

  EXECUTE format('SELECT nextval(%L)', 'inquiry_ref_' || key) INTO n;
  RETURN p_prefix || lpad(n::text, 4, '0');
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Carry any references already allocated by the old scheme, so the first
-- sequence-issued number cannot collide with one already handed to a visitor.
-- +goose StatementBegin
DO $$
DECLARE
  p        text;
  highest  bigint;
BEGIN
  FOREACH p IN ARRAY ARRAY['wr','cf','da','cp','jb'] LOOP
    SELECT COALESCE(MAX(SUBSTRING(reference_no FROM '[0-9]+$')::bigint), 0)
      INTO highest
      FROM inquiries
     WHERE reference_no ILIKE upper(p) || '-%';
    IF highest > 0 THEN
      EXECUTE format('SELECT setval(%L, %s)', 'inquiry_ref_' || p, highest);
    END IF;
  END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS next_inquiry_reference(text);
DROP SEQUENCE IF EXISTS inquiry_ref_wr;
DROP SEQUENCE IF EXISTS inquiry_ref_cf;
DROP SEQUENCE IF EXISTS inquiry_ref_da;
DROP SEQUENCE IF EXISTS inquiry_ref_cp;
DROP SEQUENCE IF EXISTS inquiry_ref_jb;
