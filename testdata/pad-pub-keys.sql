UPDATE portal_application_aats
SET public_key = RPAD(public_key, 64, '0');
