"""Регрессионный набор контура коммерции (set="p2"): 120 запросов, разметка по построению.

Запись: (тип, текст, freq, разметка, ожидание). Разметка — список (селектор, оценка); селектор — id позиции,
"fam:<family_id>" или функция от позиции. Итоговая оценка позиции — максимум по совпавшим селекторам.
Снятая позиция (price_mode = unavailable) получает не больше 1. Оценки: 3 — полностью соответствует, 2 — подходящая альтернатива, 1 — частично. Для совместимости:
verified → 3, conditional → 2; unknown и incompatible не размечаются (0): «неизвестно» не равно «совместимо».

`expect` — что проверяет запрос сверх выдачи (поле `expect` в queries.json, только у p2):
  anchor_family, required ([{family_id, quantity}]), no_required, existing_license, upgrade (family_id),
  mpn, filters ([{key, op, value, value2, unit}]), quantity, compat_target, incompatible / unknown (id позиций,
  которых не должно быть в блоке и в выдаче как «совместимых»), companions (null — блок не нужен).
"""


def cat(c):
    return lambda p: p["category_id"] == c


def spec(c, **cond):
    """Категория + числовые условия: key=v (eq), key=(">=", v), key=("<=", v), key=("range", a, b)."""
    def f(p):
        if p["category_id"] != c:
            return False
        s = p.get("specs", {})
        for k, v in cond.items():
            if k.startswith("a_"):
                if str(p["attributes"].get(k[2:])) != str(v):
                    return False
                continue
            x = s.get(k)
            if x is None:
                return False
            if isinstance(v, tuple):
                op = v[0]
                if op == ">=" and not x >= v[1]:
                    return False
                if op == "<=" and not x <= v[1]:
                    return False
                if op == "range" and not v[1] <= x <= v[2]:
                    return False
            elif x != v:
                return False
        return True
    return f


def price_le(c, limit, **cond):
    base = spec(c, **cond)
    return lambda p: base(p) and p.get("price_rub") is not None and p["price_rub"] <= limit


def F(key, op, value, unit="", value2=None):
    d = {"key": key, "op": op, "value": value}
    if value2 is not None:
        d["value2"] = value2
    if unit:
        d["unit"] = unit
    return d


LAPTOP, MONITOR, SWITCH, UPS = cat("laptops"), cat("monitors"), cat("switches"), cat("ups")
SERVER, MEMORY, DISK = cat("server-systems"), cat("server-memory"), cat("server-storage")
CAL25 = "fam:windows-server-cal-2025"
WIN_OS = lambda p: p["category_id"] == "laptops" and p["attributes"].get("os") == "предустановлена"
NO_OS = lambda p: p["category_id"] == "laptops" and p["attributes"].get("os") == "без ОС"
CAL25_USER = lambda p: p["family_id"] == "windows-server-cal-2025" and p["license"]["metric"] == "user"
CAL25_DEV = lambda p: p["family_id"] == "windows-server-cal-2025" and p["license"]["metric"] == "device"
NEW = lambda fam: (lambda p: p["family_id"] == fam and p["license"]["type"] != "renewal")
CAL22 = ["wscal-u1", "wscal-d1", "wscal-u5", "wscal-d5"]
COMPAT = "compat"  # разметка из таблицы совместимости: verified 3, conditional 2

P2_QUERIES = [
    # ---------------------------------------------------------------- лицензионные (20)
    ("p2_license", "windows server 2025 standard на 50 пользователей", 6,
     [("ws25-std16", 3), ("ws25-std2", 2), ("ws25-dc16", 1)],
     dict(anchor_family="windows-server-2025", quantity=50,
          required=[{"family_id": "windows-server-cal-2025", "metric": "user", "quantity": 50}],
          incompatible=CAL22 + ["wscal-rds"])),
    ("p2_license", "windows server 2025 datacenter 16 ядер", 4,
     [("ws25-dc16", 3), ("ws25-std16", 1)],
     dict(anchor_family="windows-server-2025", required=[{"family_id": "windows-server-cal-2025"}],
          filters=[F("cpu_cores", "eq", 16, "ядер")])),
    ("p2_license", "лицензия windows server 2025 на 32 ядра", 3,
     [("ws25-std16", 3), ("ws25-std2", 2), ("ws25-dc16", 2)],
     dict(anchor_family="windows-server-2025", required=[{"family_id": "windows-server-cal-2025"}])),
    ("p2_license", "cal 2025 на 25 пользователей", 5,
     [("wscal25-u1", 3), ("wscal25-u5", 3), ("wscal25-d1", 1), ("wscal25-d5", 1)],
     dict(quantity=25, incompatible=CAL22)),
    ("p2_license", "клиентские лицензии windows server 2025 на устройство", 3,
     [("wscal25-d1", 3), ("wscal25-d5", 3), ("wscal25-u1", 1), ("wscal25-u5", 1)], dict(incompatible=CAL22)),
    ("p2_license", "rds cal 2025", 4, [("rdscal25-u1", 3), ("rdscal25-d1", 3), ("wscal-rds", 1)], dict()),
    ("p2_license", "удаленный рабочий стол windows server 2025 на 10 пользователей", 3,
     [("rdscal25-u1", 3), ("rdscal25-d1", 2), ("ws25-std16", 1)],
     dict(anchor_family="windows-server-2025", quantity=10,
          required=[{"family_id": "rds-cal-2025", "metric": "user", "quantity": 10}], incompatible=["wscal-rds"])),
    ("p2_license", "cal для windows server 2022 на 5 устройств", 3,
     [("wscal-d5", 3), ("wscal-d1", 3), ("wscal25-d5", 2), ("wscal25-d1", 2), ("wscal-u5", 1), ("wscal-u1", 1)],
     dict(quantity=5)),
    ("p2_license", "windows server 2022 standard на 20 пользователей", 4,
     [("ws22-std16", 3), ("ws22-std2", 2), ("ws22-dc16", 1)],
     dict(anchor_family="windows-server-2022", quantity=20,
          required=[{"family_id": "windows-server-cal", "metric": "user", "quantity": 20}])),
    ("p2_license", "sql server 2022 standard server cal на 15 пользователей", 3,
     [("sql22-stdsrv", 3), ("sql22-cal", 2), ("sql22-std2c", 1)],
     dict(anchor_family="sql-server-2022", quantity=15,
          required=[{"product_id": "sql22-cal", "quantity": 15}])),
    ("p2_license", "sql server 2022 standard по ядрам на 4 ядра", 2,
     [("sql22-std2c", 3), ("sql22-stdsrv", 1)],
     dict(anchor_family="sql-server-2022", no_required=True, filters=[F("cpu_cores", "eq", 4, "ядер")])),
    ("p2_license", "sql cal 2022 на пользователя", 2, [("sql22-cal", 3), ("sql22-stdsrv", 1)], dict()),
    ("p2_license", "1с сервер корп и клиентские лицензии на 20 пользователей", 2,
     [("1csrv-corp64", 3), ("1ccl-20", 3), ("1csrv-prof64", 1), ("1ccl-10", 1), ("1ccl-50", 1), ("1ccl-1", 2)],
     dict(anchor_family="1c-server", quantity=20,
          required=[{"family_id": "1c-client-license", "quantity": 20}])),
    ("p2_license", "сервер 1с проф 64 бит", 3,
     [("1csrv-prof64", 3), ("1csrv-corp64", 1), ("1csrv-prof32", 1)],
     dict(anchor_family="1c-server", required=[{"family_id": "1c-client-license"}])),
    ("p2_license", "office 2024 для дома и бизнеса 10 пк", 3,
     [("off24-hb", 3), ("off24-home", 1), ("off21-hb", 1)], dict(anchor_family="office-2024", no_required=True)),
    ("p2_license", "microsoft 365 business premium на 30 пользователей", 3,
     [("m365b-bp", 3), ("m365b-bs", 1), ("m365b-e3", 1)],
     dict(anchor_family="microsoft-365-business", no_required=True, quantity=30)),
    ("p2_license", "microsoft 365 business standard годовая подписка", 4,
     [("m365b-bs", 3), ("m365b-bp", 1), ("m365b-bb", 1)],
     dict(anchor_family="microsoft-365-business", no_required=True, upgrade="m365b-bp")),
    ("p2_license", "windows server 2025 standard cal уже есть", 2,
     [("ws25-std16", 3), ("ws25-std2", 2)],
     dict(anchor_family="windows-server-2025", existing_license=True,
          required=[{"family_id": "windows-server-cal-2025"}])),
    ("p2_license", "kaspersky endpoint security стандартный 25 узлов", 3,
     [("kes-std25", 3), ("kes-std10", 1), ("kes-std50", 1), ("kes-ext10", 1)],
     dict(anchor_family="kaspersky-endpoint-security", no_required=True, upgrade="kes-ext10")),
    ("p2_license", "windows server 2025 на 50 устройств", 3,
     [("ws25-std16", 3), ("ws25-std2", 2), ("ws25-dc16", 1)],
     dict(anchor_family="windows-server-2025", quantity=50,
          required=[{"family_id": "windows-server-cal-2025", "metric": "device", "quantity": 50}],
          incompatible=CAL22)),

    # ---------------------------------------------------------------- парт-номера и модели (20)
    ("p2_mpn", "P00924-B21", 3, [("mem-hpe-32g-d4", 3)], dict(mpn="P00924-B21")),
    ("p2_mpn", "p00924b21", 1, [("mem-hpe-32g-d4", 3)], dict(mpn="P00924-B21")),
    ("p2_mpn", "P43328-B21", 2, [("mem-hpe-32g-d5", 3)], dict(mpn="P43328-B21")),
    ("p2_mpn", "370-AEVQ", 2, [("mem-dell-32g-d4", 3)], dict(mpn="370-AEVQ")),
    ("p2_mpn", "370aevq", 1, [("mem-dell-32g-d4", 3)], dict(mpn="370-AEVQ")),
    ("p2_mpn", "KSM32RD4/32HDR", 2, [("mem-kingston-32g-d4", 3)], dict(mpn="KSM32RD4/32HDR")),
    ("p2_mpn", "ksm32rd4 32hdr", 1, [("mem-kingston-32g-d4", 3)], dict(mpn="KSM32RD4/32HDR")),
    ("p2_mpn", "MZ7L3960HCJR-00A07", 1, [("ssd-samsung-pm893-960", 3)], dict(mpn="MZ7L3960HCJR-00A07")),
    ("p2_mpn", "ST8000NM000A", 2, [("hdd-seagate-exos-8tb", 3)], dict(mpn="ST8000NM000A")),
    ("p2_mpn", "881457-B21", 2, [("hdd-hpe-2400-sas", 3)], dict(mpn="881457-B21")),
    ("p2_mpn", "JL253A", 1, [("sw-aruba-2930f-24", 3)], dict(mpn="JL253A")),
    ("p2_mpn", "MES2324B", 3, [("sw-eltex-mes2324b", 3), ("sw-eltex-mes2324p", 1)], dict(mpn="MES2324B")),
    ("p2_mpn", "S5735-L24T4S-A1", 1, [("sw-huawei-s5735-24", 3)], dict(mpn="S5735-L24T4S-A1")),
    ("p2_mpn", "dell poweredge r740", 5,
     [("fam:dell-poweredge-r740", 3), ("fam:dell-poweredge-r750", 1)], dict(model_id="dell-poweredge-r740")),
    ("p2_mpn", "r750", 3, [("fam:dell-poweredge-r750", 3)], dict(model_id="dell-poweredge-r750")),
    ("p2_mpn", "hpe dl380 gen11", 4,
     [("fam:hpe-proliant-dl380-gen11", 3), ("fam:hpe-proliant-dl380-gen10", 1)],
     dict(model_id="hpe-proliant-dl380-gen11")),
    ("p2_mpn", "thinkpad t14 gen 4", 3,
     [("fam:lenovo-thinkpad-t14-gen4", 3), ("fam:lenovo-thinkpad-e14-gen5", 1)],
     dict(model_id="lenovo-thinkpad-t14-gen4")),
    ("p2_mpn", "P2425H", 2, [("mon-dell-p2425h", 3)], dict(mpn="P2425H")),
    ("p2_mpn", "Р00924-В21", 1, [("mem-hpe-32g-d4", 3)],
     dict(mpn="P00924-B21", notes_extra="кириллические Р и В вместо латинских")),
    ("p2_mpn", "P0092", 1, [("mem-hpe-16g-d4", 3), ("mem-hpe-32g-d4", 3), ("mem-hpe-64g-d4", 3)],
     dict(mpn_prefix="P0092")),

    # ---------------------------------------------------------------- числовые характеристики (20)
    ("p2_specs", "ноутбук 16 гб 512 гб", 5, [(spec("laptops", ram_gb=16, storage_gb=512), 3)],
     dict(filters=[F("ram_gb", "eq", 16, "ГБ"), F("storage_gb", "eq", 512, "ГБ")])),
    ("p2_specs", "ноутбук 15,6 дюйма до 100000 рублей", 4,
     [(price_le("laptops", 100000, screen_inch=15.6), 3), (spec("laptops", screen_inch=15.6), 1)],
     dict(filters=[F("screen_inch", "eq", 15.6, "″"), F("price_rub", "lte", 100000, "₽")])),
    ("p2_specs", "20 ноутбуков с 16 гб оперативной памяти", 3, [(spec("laptops", ram_gb=16), 3)],
     dict(quantity=20, filters=[F("ram_gb", "eq", 16, "ГБ")])),
    ("p2_specs", "ноутбук от 32 гб озу", 3, [(spec("laptops", ram_gb=(">=", 32)), 3)],
     dict(filters=[F("ram_gb", "gte", 32, "ГБ")])),
    ("p2_specs", "ноутбук 14 дюймов 1 тб", 2, [(spec("laptops", screen_inch=14.0, storage_gb=1000), 3),
                                             (spec("laptops", screen_inch=14.0), 1)],
     dict(filters=[F("screen_inch", "eq", 14, "″"), F("storage_gb", "eq", 1000, "ГБ")])),
    ("p2_specs", "ноутбук без операционной системы", 4, [(NO_OS, 3)],
     dict(filters=[F("os", "eq", "без ОС")])),
    ("p2_specs", "монитор 27 дюймов", 5, [(spec("monitors", screen_inch=27.0), 3)],
     dict(filters=[F("screen_inch", "eq", 27, "″")])),
    ("p2_specs", "монитор 24-27 дюймов", 3,
     [(spec("monitors", screen_inch=("range", 24, 27)), 3), (spec("monitors", screen_inch=23.8), 1)],
     dict(filters=[F("screen_inch", "range", 24, "″", 27)])),
    ("p2_specs", "монитор 144 гц", 3, [(spec("monitors", refresh_hz=(">=", 144)), 3)],
     dict(filters=[F("refresh_hz", "gte", 144, "Гц")])),
    ("p2_specs", "монитор 27\" не менее 100 гц", 2,
     [(spec("monitors", screen_inch=27.0, refresh_hz=(">=", 100)), 3), (spec("monitors", screen_inch=27.0), 1)],
     dict(filters=[F("screen_inch", "eq", 27, "″"), F("refresh_hz", "gte", 100, "Гц")])),
    ("p2_specs", "коммутатор 48 портов", 4, [(spec("switches", ports_count=48), 3)],
     dict(filters=[F("ports_count", "eq", 48, "портов")])),
    ("p2_specs", "коммутатор 24 порта poe", 3,
     [(spec("switches", ports_count=24, a_poe=True), 3), (spec("switches", ports_count=48, a_poe=True), 1)],
     dict(filters=[F("ports_count", "eq", 24, "портов"), F("poe", "eq", True)])),
    ("p2_specs", "коммутатор 24 порта аплинк 10 гбит/с", 2,
     [(spec("switches", ports_count=24, net_gbps=10), 3), (spec("switches", ports_count=24), 1)],
     dict(filters=[F("ports_count", "eq", 24, "портов"), F("net_gbps", "eq", 10, "Гбит/с")])),
    ("p2_specs", "серверная память 32 гб ddr4", 4,
     [(spec("server-memory", ram_gb=32, a_memory_type="DDR4"), 3), (spec("server-memory", ram_gb=32), 1)],
     dict(filters=[F("ram_gb", "eq", 32, "ГБ"), F("memory_type", "eq", "DDR4")])),
    ("p2_specs", "память 64 гб ddr5", 2, [(spec("server-memory", ram_gb=64, a_memory_type="DDR5"), 3),
                                         (spec("server-memory", ram_gb=64), 1)],
     dict(filters=[F("ram_gb", "eq", 64, "ГБ"), F("memory_type", "eq", "DDR5")])),
    ("p2_specs", "ssd 960 гб sata", 4, [(spec("server-storage", storage_gb=960, a_drive_type="SSD",
                                             a_interface="SATA"), 3),
                                        (spec("server-storage", storage_gb=960, a_drive_type="SSD"), 1)],
     dict(filters=[F("storage_gb", "eq", 960, "ГБ"), F("drive_type", "eq", "SSD"), F("interface", "eq", "SATA")])),
    ("p2_specs", "жесткий диск 8 тб", 3, [(spec("server-storage", storage_gb=8000, a_drive_type="HDD"), 3)],
     dict(filters=[F("storage_gb", "eq", 8000, "ГБ"), F("drive_type", "eq", "HDD")])),
    ("p2_specs", "ssd от 1,92 тб", 2, [(spec("server-storage", storage_gb=(">=", 1920), a_drive_type="SSD"), 3)],
     dict(filters=[F("storage_gb", "gte", 1920, "ГБ"), F("drive_type", "eq", "SSD")])),
    ("p2_specs", "ибп от 2 квт", 2, [(spec("ups", power_w=(">=", 2000)), 3)],
     dict(filters=[F("power_w", "gte", 2000, "Вт")])),
    ("p2_specs", "сервер 2u от 128 гб памяти", 3, [(spec("server-systems", form_factor_u=2, ram_gb=(">=", 128)), 3),
                                                  (spec("server-systems", form_factor_u=2), 1)],
     dict(filters=[F("form_factor_u", "eq", 2, "U"), F("ram_gb", "gte", 128, "ГБ")])),

    # ---------------------------------------------------------------- совместимость (25)
    ("p2_compat", "память для dell r740", 6, [(COMPAT, "server-memory")], dict(compat_target="dell-poweredge-r740")),
    ("p2_compat", "память 32 гб для dell poweredge r750", 4, [(COMPAT, "server-memory", dict(ram_gb=32))],
     dict(compat_target="dell-poweredge-r750", filters=[F("ram_gb", "eq", 32, "ГБ")])),
    ("p2_compat", "оперативная память для hpe dl380 gen10", 4, [(COMPAT, "server-memory")],
     dict(compat_target="hpe-proliant-dl380-gen10")),
    ("p2_compat", "память ddr5 для proliant dl380 gen11", 3, [(COMPAT, "server-memory")],
     dict(compat_target="hpe-proliant-dl380-gen11", filters=[F("memory_type", "eq", "DDR5")])),
    ("p2_compat", "ssd для r740", 4, [(COMPAT, "server-storage", dict(a_drive_type="SSD"))],
     dict(compat_target="dell-poweredge-r740", filters=[F("drive_type", "eq", "SSD")])),
    ("p2_compat", "диски для hpe dl380 gen11", 3, [(COMPAT, "server-storage")],
     dict(compat_target="hpe-proliant-dl380-gen11")),
    ("p2_compat", "жесткий диск sas для dell r750", 2, [(COMPAT, "server-storage", dict(a_drive_type="HDD",
                                                                                     a_interface="SAS"))],
     dict(compat_target="dell-poweredge-r750", filters=[F("drive_type", "eq", "HDD"), F("interface", "eq", "SAS")])),
    ("p2_compat", "память для yadro vegman r220", 2, [(COMPAT, "server-memory")],
     dict(compat_target="yadro-vegman-r220-g2")),
    ("p2_compat", "гарантия для poweredge r740", 3, [(COMPAT, "server-services")],
     dict(compat_target="dell-poweredge-r740")),
    ("p2_compat", "продление гарантии hpe dl380 gen10", 2, [(COMPAT, "server-services")],
     dict(compat_target="hpe-proliant-dl380-gen10")),
    ("p2_compat", "ddr5 для dell r740", 2, [(COMPAT, "server-memory", dict(a_memory_type="DDR5"))],
     dict(compat_target="dell-poweredge-r740", filters=[F("memory_type", "eq", "DDR5")])),
    ("p2_compat", "64 гб для r750", 2, [(COMPAT, "server-memory", dict(ram_gb=64))],
     dict(compat_target="dell-poweredge-r750", filters=[F("ram_gb", "eq", 64, "ГБ")])),
    ("p2_compat", "kingston для dell r750", 2,
     [(COMPAT, "server-memory", dict(a_module="RDIMM"), "kingston")], dict(compat_target="dell-poweredge-r750")),
    ("p2_compat", "ssd 960 для hpe gen10", 2, [(COMPAT, "server-storage", dict(storage_gb=960, a_drive_type="SSD"))],
     dict(compat_target="hpe-proliant-dl380-gen10", filters=[F("storage_gb", "eq", 960, "ГБ")])),
    ("p2_compat", "nvme для dl380 gen11", 2, [(COMPAT, "server-storage", dict(a_interface="NVMe"))],
     dict(compat_target="hpe-proliant-dl380-gen11", filters=[F("interface", "eq", "NVMe")])),
    ("p2_compat", "диск 3,5 8 тб для dl380 gen11", 1,
     [(COMPAT, "server-storage", dict(storage_gb=8000, a_drive_form_factor="3.5"))],
     dict(compat_target="hpe-proliant-dl380-gen11", filters=[F("storage_gb", "eq", 8000, "ГБ")])),
    ("p2_compat", "память 16 гб для poweredge r740", 2, [(COMPAT, "server-memory", dict(ram_gb=16))],
     dict(compat_target="dell-poweredge-r740", filters=[F("ram_gb", "eq", 16, "ГБ")])),
    ("p2_compat", "cal 2025 для windows server 2025", 3, [(CAL25, 3)],
     dict(compat_target="windows-server-2025", incompatible=CAL22)),
    ("p2_compat", "cal для windows server 2025", 5, [(CAL25, 3)],
     dict(compat_target="windows-server-2025", incompatible=CAL22 + ["wscal-rds"])),
    ("p2_compat", "rds cal для windows server 2025", 3, [("fam:rds-cal-2025", 3)],
     dict(compat_target="windows-server-2025", incompatible=["wscal-rds"])),
    ("p2_compat", "rds cal 2022 для windows server 2025", 1, [("fam:rds-cal-2025", 2)],
     dict(compat_target="windows-server-2025", incompatible=["wscal-rds"])),
    ("p2_compat", "kaspersky для astra linux", 3,
     [(lambda p: p["family_id"] == "kaspersky-endpoint-security" and p["license"]["type"] != "renewal", 3),
      (lambda p: p["family_id"] == "kaspersky-endpoint-security", 2)],
     dict(compat_target="astra-linux-se")),
    ("p2_compat", "мойофис для astra linux", 2, [("fam:myoffice-standard", 3)], dict(compat_target="astra-linux-se")),
    ("p2_compat", "samsung pm893 для dell r740", 1, [],
     dict(compat_target="dell-poweredge-r740", unknown=["ssd-samsung-pm893-960"])),
    ("p2_compat", "kingston ddr5 для dl380 gen11", 1,
     [(COMPAT, "server-memory", dict(a_memory_type="DDR5"), "kingston")],
     dict(compat_target="hpe-proliant-dl380-gen11", filters=[F("memory_type", "eq", "DDR5")])),

    # ---------------------------------------------------------------- комплексные закупки (15)
    ("p2_bundle", "рабочее место для 20 сотрудников", 4,
     [(LAPTOP, 2), (MONITOR, 1), ("m365b-bs", 2), ("kes-std25", 2)], dict(quantity=20)),
    ("p2_bundle", "рабочее место бухгалтера", 3,
     [(LAPTOP, 2), ("1cbuh-prof", 2), ("off24-hb", 2), (MONITOR, 1)], dict()),
    ("p2_bundle", "оснастить офис на 10 человек ноутбуками и офисом", 2,
     [(LAPTOP, 3), ("fam:microsoft-365-business", 2), ("fam:office-2024", 2)],
     dict(quantity=10)),
    ("p2_bundle", "сервер под 1с на 50 пользователей", 4,
     [(spec("server-systems", ram_gb=(">=", 128)), 2), ("fam:1c-server", 2), ("1ccl-50", 2), ("fam:postgres-pro", 1),
      ("ws25-std16", 1)], dict(quantity=50)),
    ("p2_bundle", "сервер для 1с с windows server и sql", 2,
     [(SERVER, 2), ("ws25-std16", 2), ("sql22-stdsrv", 2), ("fam:1c-server", 1)],
     dict()),
    ("p2_bundle", "удаленный офис на 5 человек", 2,
     [(SWITCH, 2), (UPS, 2), (LAPTOP, 2), ("m365b-bb", 2)], dict()),
    ("p2_bundle", "филиал на 15 рабочих мест", 2,
     [(SWITCH, 2), (UPS, 2), (LAPTOP, 2)], dict()),
    ("p2_bundle", "комплект для нового сотрудника ноутбук офис антивирус", 2,
     [(LAPTOP, 3), ("fam:microsoft-365-business", 2), ("fam:office-2024", 2), (NEW("kaspersky-endpoint-security"), 2)],
     dict(quantity=1)),
    ("p2_bundle", "ноутбук с windows и office", 3, [(WIN_OS, 3), (NO_OS, 1), ("fam:office-2024", 2)],
     dict(companions=True, filters=[F("os", "eq", "предустановлена")])),
    ("p2_bundle", "сервер dell r750 с памятью и дисками", 2,
     [("fam:dell-poweredge-r750", 3), (COMPAT, "server-memory", None, None, "dell-poweredge-r750"),
      (COMPAT, "server-storage", None, None, "dell-poweredge-r750")],
     dict(anchor_family="dell-poweredge-r750", compat_target="dell-poweredge-r750")),
    ("p2_bundle", "windows server 2025 с cal на 30 пользователей", 3,
     [("ws25-std16", 3), (CAL25_USER, 3), ("ws25-std2", 1)],
     dict(anchor_family="windows-server-2025", quantity=30,
          required=[{"family_id": "windows-server-cal-2025", "metric": "user", "quantity": 30}])),
    ("p2_bundle", "рабочее место на отечественном по", 2,
     [("astra-orel", 2), ("astra-vrn", 2), ("astra-smol", 2), ("fam:myoffice-standard", 2),
      ("fam:aquarius-cmp-ns685", 2), (NEW("kaspersky-endpoint-security"), 1)], dict()),
    ("p2_bundle", "коммутатор и ибп для небольшого офиса", 2, [(SWITCH, 2), (UPS, 2)],
     dict()),
    ("p2_bundle", "сервер hpe gen11 под виртуализацию", 2,
     [("fam:hpe-proliant-dl380-gen11", 3), ("ws25-dc16", 2), ("fam:vmware-vsphere", 2)],
     dict(anchor_family="hpe-proliant-dl380-gen11")),
    ("p2_bundle", "закупка 10 ноутбуков и мониторов", 2, [(LAPTOP, 3), (MONITOR, 3)], dict(quantity=10)),

    # ---------------------------------------------------------------- отрицательные и пограничные (20)
    ("p2_negative", "антивирус для компании на 50 пк", 3,
     [("kes-std50", 3), ("fam:kaspersky-endpoint-security", 1), ("dress-cx50", 3)],
     dict(companions=None, quantity=50)),
    ("p2_negative", "серверная память", 3, [(MEMORY, 1)], dict(companions=None)),
    ("p2_negative", "dell", 2, [(SERVER, 1), (LAPTOP, 1), (MONITOR, 1)], dict(companions=None)),
    ("p2_negative", "ddr4", 2, [(spec("server-memory", a_memory_type="DDR4"), 2)], dict(companions=None)),
    ("p2_negative", "32", 1, [], dict(companions=None, filters=[])),
    ("p2_negative", "16 гб", 2, [(spec("laptops", ram_gb=16), 1), (spec("server-memory", ram_gb=16), 1)],
     dict(companions=None)),
    ("p2_negative", "cal", 3, [("fam:windows-server-cal", 2), (CAL25, 2), ("fam:rds-cal-2025", 1), ("sql22-cal", 1)],
     dict(companions=None)),
    ("p2_negative", "windows server без cal", 2, [("ws25-std16", 2), ("ws22-std16", 2)],
     dict(existing_license=True)),
    ("p2_negative", "sql server 2022 enterprise на ядра", 2, [("sql22-ent2c", 3)],
     dict(anchor_family="sql-server-2022", no_required=True)),
    ("p2_negative", "10 ноутбуков", 3, [(LAPTOP, 2)], dict(quantity=10, filters=[])),
    ("p2_negative", "монитор для ноутбука", 2, [(MONITOR, 2)], dict(compat_target=None)),
    ("p2_negative", "сумка для ноутбука", 2, [], dict(companions=None, absent=True)),
    ("p2_negative", "supermicro sys-6029p", 1, [(SERVER, 1)], dict(companions=None, absent=True)),
    ("p2_negative", "iphone 15 pro", 2, [], dict(companions=None, absent=True)),
    ("p2_negative", "memory for server", 1, [(MEMORY, 2)], dict(companions=None)),
    ("p2_negative", "rds cal 2019", 1, [("wscal-rds", 1), ("fam:rds-cal-2025", 1)],
     dict(companions=None, absent=True)),
    ("p2_negative", "бесплатный антивирус", 2, [], dict(companions=None, absent=True)),
    ("p2_negative", "ram_gb:32", 1, [(spec("server-memory", ram_gb=32), 1)], dict(companions=None)),
    ("p2_negative", "windows server 2025 и cal 2022", 1, [("ws25-std16", 2), (CAL25, 2)],
     dict(anchor_family="windows-server-2025", incompatible=CAL22)),
    ("p2_negative", "p00924", 1, [("mem-hpe-32g-d4", 3)], dict(mpn_prefix="P00924")),
]
