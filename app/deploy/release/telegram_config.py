"""Fixed private Telegram JSON contracts, without printing provider data."""
import re
from urllib.parse import urlsplit
from uuid import UUID

from bundle import read_json, require


def validate_telegram_file(path, key):
    failure = 'Invalid private Telegram service credential.'
    require(path.is_file() and not path.is_symlink() and 0 < path.stat().st_size <= 8192, failure)
    raw = path.read_bytes()
    require(len(raw) <= 8192, failure)
    bots = read_json(raw)
    require(type(bots) is list and len(bots) <= 8 and (key == 'CP_TELEGRAM_INGRESS' or len(bots) >= 1), failure)
    fields = {'bot_id', 'organization_id'}
    if key == 'CP_TELEGRAM_INGRESS':
        fields |= {'username', 'webhook_secret'}
    elif key == 'CP_TELEGRAM_DELIVERY_BOTS':
        fields |= {'token', 'portal_origin'}
    else:
        require(key == 'CP_TELEGRAM_DISPATCH_BOTS', failure)
    seen, names, secrets = set(), set(), set()
    for bot in bots:
        require(type(bot) is dict and set(bot) == fields and all(type(v) is str for v in bot.values()), failure)
        identity = bot['bot_id']
        require(re.fullmatch(r'[1-9][0-9]{0,15}', identity) and int(identity) <= (1 << 52) - 1 and identity not in seen, failure)
        seen.add(identity)
        try:
            organization = UUID(bot['organization_id'])
        except ValueError:
            require(False, failure)
        require(str(organization) == bot['organization_id'], failure)
        if key == 'CP_TELEGRAM_INGRESS':
            name, secret = bot['username'].lower(), bot['webhook_secret']
            require(re.fullmatch(r'[A-Za-z][A-Za-z0-9_]{4,31}', bot['username']) and name.endswith('bot')
                    and re.fullmatch(r'[A-Za-z0-9_-]{32,256}', secret) and name not in names and secret not in secrets, failure)
            names.add(name)
            secrets.add(secret)
        elif key == 'CP_TELEGRAM_DELIVERY_BOTS':
            require(re.fullmatch(re.escape(identity) + r':[A-Za-z0-9_-]{30,128}', bot['token']), failure)
            try:
                origin = urlsplit(bot['portal_origin'])
                valid = (origin.scheme == 'https' and origin.netloc and origin.hostname and not origin.username
                         and not origin.password and not origin.path and not origin.query and not origin.fragment
                         and bot['portal_origin'] == 'https://' + origin.netloc
                         and not any(ord(c) <= 32 or c in '\\' for c in bot['portal_origin']))
            except ValueError:
                valid = False
            require(valid, failure)
    return bots
