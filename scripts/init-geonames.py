"""Build the optional weather city catalog; Python 3.10+, standard library only.

GeoNames CC BY 4.0: https://download.geonames.org/export/dump/readme.txt
Raw archives and the derived catalog stay under ignored data/cities/.
"""
import argparse
import datetime
import hashlib
import io
import json
import math
from pathlib import Path
import urllib.request
import unicodedata
import zipfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--directory', type=Path, default=Path('data/cities'))
    parser.add_argument('--refresh', action='store_true', help='explicitly replace downloaded source snapshots')
    parser.add_argument('--proxy', default='', help='explicit HTTP(S) download proxy; no environment proxy is read')
    args = parser.parse_args()
    raw = args.directory / 'raw'
    raw.mkdir(parents=True, exist_ok=True)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({'http': args.proxy, 'https': args.proxy} if args.proxy else {}))
    sources = {}
    for name in ['cities5000.zip', 'admin1CodesASCII.txt', 'alternateNamesV2.zip']:
        path = raw / name
        if args.refresh or not path.exists():
            print(f'Downloading GeoNames {name}', flush=True)
            request = urllib.request.Request('https://download.geonames.org/export/dump/' + name, headers={'User-Agent': 'GoFurry-city-init/1'})
            temporary = path.with_suffix(path.suffix + '.part')
            with opener.open(request, timeout=60) as response, temporary.open('wb') as output:
                total = 0
                while chunk := response.read(1024 * 1024):
                    total += len(chunk)
                    if total > 300 * 1024 * 1024:
                        raise ValueError('Source exceeds download bound')
                    output.write(chunk)
            temporary.replace(path)
        digest = hashlib.sha256()
        with path.open('rb') as source:
            for chunk in iter(lambda: source.read(1024 * 1024), b''):
                digest.update(chunk)
        sources[name] = digest.hexdigest()
    admins = {}
    with (raw / 'admin1CodesASCII.txt').open(encoding='utf-8') as source:
        for line in source:
            fields = line.rstrip('\n').split('\t')
            if len(fields) == 4:
                admins[fields[0]] = fields[1]
    cities = {}
    with zipfile.ZipFile(raw / 'cities5000.zip') as archive, archive.open('cities5000.txt') as source:
        for line in io.TextIOWrapper(source, encoding='utf-8'):
            f = line.rstrip('\n').split('\t')
            if len(f) != 19 or f[6] != 'P':
                raise ValueError('Unexpected cities5000 schema')
            lat, lon = float(f[4]), float(f[5])
            if not math.isfinite(lat) or not math.isfinite(lon) or not -90 <= lat <= 90 or not -180 <= lon <= 180:
                raise ValueError('Invalid city coordinates')
            aliases = sorted({name for name in [f[1], f[2], *f[3].split(',')] if 0 < len(name) <= 256 and not any(unicodedata.category(ch) == 'Cc' for ch in name)})
            cities[f[0]] = dict(city_id=f[0], name=f[1], names=dict(zh=f[1], en=f[2] or f[1]),
                                aliases=aliases[:512], country_code=f[8], admin=admins.get(f'{f[8]}.{f[10]}', f[10]),
                                latitude=round(lat, 2), longitude=round(lon, 2), population=int(f[14]))
    # Read only explicit language labels. Never infer Chinese from CJK glyphs.
    chosen = {}
    with zipfile.ZipFile(raw / 'alternateNamesV2.zip') as archive, archive.open('alternateNamesV2.txt') as source:
        for line in io.TextIOWrapper(source, encoding='utf-8'):
            f = line.rstrip('\n').split('\t')
            if len(f) < 8 or f[1] not in cities or f[2] not in ('zh', 'zh-CN', 'zh-Hans', 'en') or f[7] == '1' or f[6] == '1' or not 0 < len(f[3]) <= 256:
                continue
            lang = 'en' if f[2] == 'en' else 'zh'
            score = (f[2] in ('zh-CN', 'zh-Hans'), f[4] == '1', f[5] == '1')
            key = (f[1], lang)
            if key not in chosen or score > chosen[key]:
                chosen[key] = score
                cities[f[1]]['names'][lang] = f[3]
    catalog = dict(format=1, source='GeoNames cities5000', license='CC BY 4.0',
                   generated_at=datetime.datetime.now(datetime.timezone.utc).isoformat(), sources=sources,
                   cities=sorted(cities.values(), key=lambda city: int(city['city_id'])))
    output = args.directory / 'cities.json'
    temporary = output.with_suffix('.json.part')
    temporary.write_text(json.dumps(catalog, ensure_ascii=False, separators=(',', ':')), encoding='utf-8')
    temporary.replace(output)
    print(f'Built {len(cities)} cities: {output} ({output.stat().st_size} bytes)', flush=True)


if __name__ == '__main__':
    main()
