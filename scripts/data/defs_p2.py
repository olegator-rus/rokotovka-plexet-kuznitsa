"""Контур коммерции: оборудование, лицензии доступа 2025, правила, шаблоны решений.

Названия моделей реальные, парт-номера правдоподобные
(часть совпадает с вендорскими, часть сгенерирована по шаблону вендора), цены — ориентиры.
"""

MS_CAL_URL = "https://www.microsoft.com/en-us/licensing/product-licensing/client-access-license.aspx"
MS_RDS_URL = "https://learn.microsoft.com/windows-server/remote/remote-desktop-services/rds-client-access-license"
MS_SQL_URL = "https://www.microsoft.com/en-us/sql-server/sql-server-2022-pricing"
ONEC_URL = "https://v8.1c.ru/buy/"
DELL_URL = "https://www.dell.com/support/home/ru-ru/product-support/product/{model}/docs"
HPE_URL = "https://support.hpe.com/connect/s/product?language=ru&kmpmoid={model}"
YADRO_URL = "https://yadro.com/ru/vegman"
KINGSTON_URL = "https://www.kingston.com/ru/memory/server"
REVIEWED = "2026-05-22"

VENDORS_P2 = [
    ("dell", "Dell Technologies", "foreign"),
    ("hpe", "Hewlett Packard Enterprise", "foreign"),
    ("lenovo", "Lenovo", "foreign"),
    ("hp", "HP", "foreign"),
    ("huawei", "Huawei", "foreign"),
    ("kingston", "Kingston", "foreign"),
    ("samsung", "Samsung", "foreign"),
    ("seagate", "Seagate", "foreign"),
    ("dlink", "D-Link", "foreign"),
    ("eaton", "Eaton", "foreign"),
    ("ippon", "Ippon", "foreign"),
    ("yadro", "YADRO", "ru"),
    ("aquarius", "Аквариус", "ru"),
    ("eltex", "Элтекс", "ru"),
]

CATEGORIES_P2 = [
    ("servers", "Серверы и комплектующие", None),
    ("server-systems", "Серверы", "servers"),
    ("server-memory", "Серверная память", "servers"),
    ("server-storage", "Серверные диски", "servers"),
    ("server-services", "Гарантия и сервис", "servers"),
    ("workplace-hw", "Рабочие места", None),
    ("laptops", "Ноутбуки", "workplace-hw"),
    ("monitors", "Мониторы", "workplace-hw"),
    ("network", "Сеть и питание", None),
    ("switches", "Коммутаторы", "network"),
    ("ups", "Источники бесперебойного питания", "network"),
]

R740, R750 = "dell-poweredge-r740", "dell-poweredge-r750"
G10, G11 = "hpe-proliant-dl380-gen10", "hpe-proliant-dl380-gen11"
VEG = "yadro-vegman-r220-g2"
SERVER_MODELS = [R740, R750, G10, G11, VEG]

# ------------------------------------------------------------------ ПО: лицензии доступа 2025
# (id, family_id, family_name, title, sku, edition, metric, qty, price, purchase_by_one)
SOFTWARE_P2 = [
    ("wscal25-u1", "windows-server-cal-2025", "Microsoft Windows Server 2025 CAL",
     "Microsoft Windows Server 2025 CAL, клиентская лицензия на 1 пользователя", "R18-07473", "User CAL", "user", 1,
     5790, True),
    ("wscal25-d1", "windows-server-cal-2025", "Microsoft Windows Server 2025 CAL",
     "Microsoft Windows Server 2025 CAL, клиентская лицензия на 1 устройство", "R18-07455", "Device CAL", "device", 1,
     5190, True),
    ("wscal25-u5", "windows-server-cal-2025", "Microsoft Windows Server 2025 CAL",
     "Microsoft Windows Server 2025 CAL, 5 клиентских лицензий на пользователя", "R18-07482", "User CAL", "user", 5,
     27490, False),
    ("wscal25-d5", "windows-server-cal-2025", "Microsoft Windows Server 2025 CAL",
     "Microsoft Windows Server 2025 CAL, 5 клиентских лицензий на устройство", "R18-07464", "Device CAL", "device", 5,
     24290, False),
    ("rdscal25-u1", "rds-cal-2025", "Microsoft Remote Desktop Services 2025 CAL",
     "Microsoft Remote Desktop Services 2025 CAL, клиентская лицензия на 1 пользователя", "6VC-04788", "RDS CAL", "user",
     1, 13990, True),
    ("rdscal25-d1", "rds-cal-2025", "Microsoft Remote Desktop Services 2025 CAL",
     "Microsoft Remote Desktop Services 2025 CAL, клиентская лицензия на 1 устройство", "6VC-04770", "RDS CAL",
     "device", 1, 12490, True),
]
SOFTWARE_DESC = {
    "windows-server-cal-2025": "Клиентская лицензия доступа к Windows Server 2025. Нужна каждому {unit}, который "
                               "обращается к серверу, если у компании нет действующих CAL той же или более новой "
                               "версии. Подходит и для серверов Windows Server 2022 и более ранних.",
    "rds-cal-2025": "Лицензия на подключение к удалённому рабочему столу Windows Server 2025 для одного {unit}. "
                    "Покупается в дополнение к обычной CAL. RDS CAL прежних версий к серверу 2025 не подходят.",
}

# Версия у серверных ОС, CAL и СУБД контура поиска (только добавление ключа attributes.version).
P1_VERSION = {"windows-server-2022": "2022", "windows-server-2025": "2025", "windows-server-cal": "2022",
              "sql-server-2022": "2022"}

# ------------------------------------------------------------------ оборудование
# Каждая запись — dict; общие поля дополняет сборщик в gen_mock_p2.py.
def _srv(pid, fam, fname, vendor, title, mpn, price, mode, specs, attrs, desc, model):
    return dict(id=pid, family_id=fam, family_name=fname, vendor_id=vendor, category_id="server-systems",
                title=title, mpn=mpn, price=price, price_mode=mode, specs=specs, attributes=attrs, description=desc,
                model_id=model)


SERVERS = [
    _srv("srv-r740-s4210", R740, "Dell PowerEdge R740", "dell",
         "Сервер Dell PowerEdge R740, 2× Xeon Silver 4210R, 64 ГБ, 2× SSD 960 ГБ", "PER740-2S4210R-64G", 689000,
         "fixed", dict(form_factor_u=2, cpu_cores=20, ram_gb=64, storage_gb=1920, power_w=750),
         dict(cpu="2× Intel Xeon Silver 4210R", memory_type="DDR4", drive_bays="8× 2,5\"", raid="PERC H730P"),
         "Двухпроцессорный сервер 2U для виртуализации и баз данных. 24 слота DDR4 RDIMM, восемь отсеков 2,5 дюйма, "
         "два блока питания 750 Вт.", R740),
    _srv("srv-r740-g6230", R740, "Dell PowerEdge R740", "dell",
         "Сервер Dell PowerEdge R740, 2× Xeon Gold 6230R, 256 ГБ, 4× SSD 1,92 ТБ", "PER740-2G6230R-256G", None,
         "quote_required", dict(form_factor_u=2, cpu_cores=52, ram_gb=256, storage_gb=7680, power_w=1100),
         dict(cpu="2× Intel Xeon Gold 6230R", memory_type="DDR4", drive_bays="16× 2,5\"", raid="PERC H740P"),
         "Старшая конфигурация R740 под нагруженные базы данных и VDI. Цена зависит от сборки и сроков поставки, "
         "рассчитывается по запросу.", R740),
    _srv("srv-r750-s4314", R750, "Dell PowerEdge R750", "dell",
         "Сервер Dell PowerEdge R750, 2× Xeon Silver 4314, 128 ГБ, 2× SSD 1,92 ТБ", "PER750-2S4314-128G", 1149000,
         "from", dict(form_factor_u=2, cpu_cores=32, ram_gb=128, storage_gb=3840, power_w=1100),
         dict(cpu="2× Intel Xeon Silver 4314", memory_type="DDR4", drive_bays="8× 2,5\"", raid="PERC H755"),
         "Сервер 2U на Xeon Scalable третьего поколения, 32 слота DDR4-3200. Цена указана за базовую сборку, итог "
         "зависит от конфигурации.", R750),
    _srv("srv-r750-g6338", R750, "Dell PowerEdge R750", "dell",
         "Сервер Dell PowerEdge R750, 2× Xeon Gold 6338, 512 ГБ, 8× SSD 3,84 ТБ", "PER750-2G6338-512G", None,
         "quote_required", dict(form_factor_u=2, cpu_cores=64, ram_gb=512, storage_gb=30720, power_w=1400),
         dict(cpu="2× Intel Xeon Gold 6338", memory_type="DDR4", drive_bays="24× 2,5\"", raid="PERC H755"),
         "Конфигурация R750 под кластер виртуализации. Поставка под проект, цена по запросу.", R750),
    _srv("srv-dl380g10-4210", G10, "HPE ProLiant DL380 Gen10", "hpe",
         "Сервер HPE ProLiant DL380 Gen10, 2× Xeon Silver 4210R, 64 ГБ, 2× SSD 960 ГБ", "P24842-B21", 598000,
         "unavailable", dict(form_factor_u=2, cpu_cores=20, ram_gb=64, storage_gb=1920, power_w=800),
         dict(cpu="2× Intel Xeon Silver 4210R", memory_type="DDR4", drive_bays="8× SFF", raid="S100i"),
         "Сервер 2U предыдущего поколения. Снят с производства, остатки распроданы: позиция осталась в каталоге для "
         "истории заказов.", G10),
    _srv("srv-dl380g10-5218", G10, "HPE ProLiant DL380 Gen10", "hpe",
         "Сервер HPE ProLiant DL380 Gen10, 2× Xeon Gold 5218, 128 ГБ, 4× SSD 960 ГБ", "P20249-B21", 845000,
         "fixed", dict(form_factor_u=2, cpu_cores=32, ram_gb=128, storage_gb=3840, power_w=800),
         dict(cpu="2× Intel Xeon Gold 5218", memory_type="DDR4", drive_bays="8× SFF", raid="P408i-a"),
         "Проверенная платформа 2U для 1С и виртуализации. 24 слота DDR4 HPE Smart Memory, контроллер P408i-a.", G10),
    _srv("srv-dl380g11-4410y", G11, "HPE ProLiant DL380 Gen11", "hpe",
         "Сервер HPE ProLiant DL380 Gen11, 2× Xeon Silver 4410Y, 64 ГБ DDR5, 2× SSD 960 ГБ", "P52534-B21", 1290000,
         "fixed", dict(form_factor_u=2, cpu_cores=24, ram_gb=64, storage_gb=1920, power_w=1000),
         dict(cpu="2× Intel Xeon Silver 4410Y", memory_type="DDR5", drive_bays="8× SFF", raid="MR408i-o"),
         "Сервер 2U на Xeon Scalable четвёртого поколения с памятью DDR5-4800. Отсеки только 2,5 дюйма.", G11),
    _srv("srv-dl380g11-5418y", G11, "HPE ProLiant DL380 Gen11", "hpe",
         "Сервер HPE ProLiant DL380 Gen11, 2× Xeon Gold 5418Y, 256 ГБ DDR5, 4× NVMe 3,84 ТБ", "P52562-B21", None,
         "quote_required", dict(form_factor_u=2, cpu_cores=48, ram_gb=256, storage_gb=15360, power_w=1600),
         dict(cpu="2× Intel Xeon Gold 5418Y", memory_type="DDR5", drive_bays="8× SFF NVMe", raid="MR416i-o"),
         "Старшая конфигурация Gen11 с NVMe-корзиной под высоконагруженные СУБД. Цена по запросу.", G11),
    _srv("srv-vegman-r220-base", VEG, "YADRO VEGMAN R220 G2", "yadro",
         "Сервер YADRO VEGMAN R220 G2, 2× Xeon Gold 6330, 256 ГБ, 2× SSD 960 ГБ", "VEG-R220G2-6330-256", 1420000,
         "from", dict(form_factor_u=2, cpu_cores=56, ram_gb=256, storage_gb=1920, power_w=1300),
         dict(cpu="2× Intel Xeon Gold 6330", memory_type="DDR4", drive_bays="12× 2,5\"", raid="программный"),
         "Российский сервер 2U, внесён в реестр российской промышленной продукции. Цена за базовую сборку.", VEG),
    _srv("srv-vegman-r220-max", VEG, "YADRO VEGMAN R220 G2", "yadro",
         "Сервер YADRO VEGMAN R220 G2, 2× Xeon Gold 6348, 1 ТБ, 8× SSD 3,84 ТБ", "VEG-R220G2-6348-1T", None,
         "quote_required", dict(form_factor_u=2, cpu_cores=56, ram_gb=1024, storage_gb=30720, power_w=1600),
         dict(cpu="2× Intel Xeon Gold 6348", memory_type="DDR4", drive_bays="24× 2,5\"", raid="программный"),
         "Максимальная конфигурация VEGMAN R220 под импортозамещение кластеров. Цена по запросу.", VEG),
]


def _mem(pid, fam, fname, vendor, title, mpn, price, gb, mtype, speed, desc):
    return dict(id=pid, family_id=fam, family_name=fname, vendor_id=vendor, category_id="server-memory", title=title,
                mpn=mpn, price=price, price_mode="fixed", specs=dict(ram_gb=gb),
                attributes=dict(memory_type=mtype, speed_mts=speed, module="RDIMM"), description=desc)


MEMORY = [
    _mem("mem-dell-16g-d4", "dell-rdimm-ddr4", "Dell RDIMM DDR4", "dell",
         "Модуль памяти Dell 16 ГБ DDR4-3200 RDIMM", "370-AEXY", 14900, 16, "DDR4", 3200,
         "Оригинальный модуль Dell для серверов PowerEdge. Одноранговый RDIMM с коррекцией ошибок."),
    _mem("mem-dell-32g-d4", "dell-rdimm-ddr4", "Dell RDIMM DDR4", "dell",
         "Модуль памяти Dell 32 ГБ DDR4-3200 RDIMM", "370-AEVQ", 24900, 32, "DDR4", 3200,
         "Оригинальный модуль Dell для PowerEdge 14-го и 15-го поколений. Двухранговый RDIMM."),
    _mem("mem-dell-64g-d4", "dell-rdimm-ddr4", "Dell RDIMM DDR4", "dell",
         "Модуль памяти Dell 64 ГБ DDR4-3200 RDIMM", "370-AEVP", 46900, 64, "DDR4", 3200,
         "Оригинальный модуль Dell большой ёмкости. Двухранговый RDIMM для плотных конфигураций."),
    _mem("mem-dell-32g-d4-2933", "dell-rdimm-ddr4", "Dell RDIMM DDR4", "dell",
         "Модуль памяти Dell 32 ГБ DDR4-2933 RDIMM", "370-AEQH", 22900, 32, "DDR4", 2933,
         "Модуль Dell для PowerEdge 14-го поколения. Частота 2933 МТ/с соответствует Xeon Scalable второго поколения."),
    _mem("mem-hpe-16g-d4", "hpe-smart-memory-ddr4", "HPE Smart Memory DDR4", "hpe",
         "Модуль памяти HPE 16 ГБ DDR4-2933 RDIMM", "P00920-B21", 16900, 16, "DDR4", 2933,
         "Модуль HPE Smart Memory для ProLiant Gen10. Прошивка HPE, работает с функциями мониторинга iLO."),
    _mem("mem-hpe-32g-d4", "hpe-smart-memory-ddr4", "HPE Smart Memory DDR4", "hpe",
         "Модуль памяти HPE 32 ГБ DDR4-2933 RDIMM", "P00924-B21", 28900, 32, "DDR4", 2933,
         "Модуль HPE Smart Memory для ProLiant Gen10. Двухранговый RDIMM."),
    _mem("mem-hpe-64g-d4", "hpe-smart-memory-ddr4", "HPE Smart Memory DDR4", "hpe",
         "Модуль памяти HPE 64 ГБ DDR4-2933 RDIMM", "P00930-B21", 52900, 64, "DDR4", 2933,
         "Модуль HPE Smart Memory большой ёмкости для ProLiant Gen10. Двухранговый RDIMM."),
    _mem("mem-hpe-16g-d5", "hpe-smart-memory-ddr5", "HPE Smart Memory DDR5", "hpe",
         "Модуль памяти HPE 16 ГБ DDR5-4800 RDIMM", "P43322-B21", 19900, 16, "DDR5", 4800,
         "Модуль HPE Smart Memory DDR5 для ProLiant Gen11. С памятью DDR4 не смешивается."),
    _mem("mem-hpe-32g-d5", "hpe-smart-memory-ddr5", "HPE Smart Memory DDR5", "hpe",
         "Модуль памяти HPE 32 ГБ DDR5-4800 RDIMM", "P43328-B21", 33900, 32, "DDR5", 4800,
         "Модуль HPE Smart Memory DDR5 для ProLiant Gen11. Двухранговый RDIMM."),
    _mem("mem-hpe-64g-d5", "hpe-smart-memory-ddr5", "HPE Smart Memory DDR5", "hpe",
         "Модуль памяти HPE 64 ГБ DDR5-4800 RDIMM", "P43331-B21", 61900, 64, "DDR5", 4800,
         "Модуль HPE Smart Memory DDR5 большой ёмкости для ProLiant Gen11. Двухранговый RDIMM."),
    _mem("mem-kingston-32g-d4", "kingston-server-premier", "Kingston Server Premier", "kingston",
         "Модуль памяти Kingston Server Premier 32 ГБ DDR4-3200 RDIMM", "KSM32RD4/32HDR", 17900, 32, "DDR4", 3200,
         "Серверная память Kingston с проверкой на платформах производителей. Совместимость с конкретной моделью "
         "сервера подтверждает Kingston, а не производитель сервера."),
    _mem("mem-kingston-32g-d5", "kingston-server-premier", "Kingston Server Premier", "kingston",
         "Модуль памяти Kingston Server Premier 32 ГБ DDR5-4800 RDIMM", "KSM48R40BD8-32HA", 21900, 32, "DDR5", 4800,
         "Серверная память Kingston DDR5. Совместимость с конкретной моделью сервера подтверждает Kingston."),
    _mem("mem-samsung-64g-d4", "samsung-rdimm", "Samsung RDIMM", "samsung",
         "Модуль памяти Samsung 64 ГБ DDR4-3200 RDIMM", "M393A8G40AB2-CWE", 31900, 64, "DDR4", 3200,
         "Серверный модуль Samsung без привязки к производителю сервера. Списка подтверждённых платформ у "
         "магазина нет."),
    _mem("mem-yadro-32g-d4", "yadro-memory", "YADRO RDIMM", "yadro",
         "Модуль памяти YADRO 32 ГБ DDR4-3200 RDIMM", "YM-RD4-32G-3200", 26900, 32, "DDR4", 3200,
         "Модуль памяти из списка совместимости VEGMAN. Поставляется с гарантией YADRO."),
]


def _disk(pid, fam, fname, vendor, title, mpn, price, gb, dtype, iface, ff, desc):
    return dict(id=pid, family_id=fam, family_name=fname, vendor_id=vendor, category_id="server-storage", title=title,
                mpn=mpn, price=price, price_mode="fixed", specs=dict(storage_gb=gb),
                attributes=dict(drive_type=dtype, interface=iface, drive_form_factor=ff), description=desc)


DISKS = [
    _disk("ssd-dell-960-sata", "dell-server-drives", "Dell диски для PowerEdge", "dell",
          "SSD Dell 960 ГБ SATA 2,5\" для PowerEdge", "400-BDUK", 39900, 960, "SSD", "SATA", "2.5",
          "Серверный SSD Dell с салазками для PowerEdge 14-го и 15-го поколений. Ресурс — интенсивное чтение."),
    _disk("ssd-dell-1920-sata", "dell-server-drives", "Dell диски для PowerEdge", "dell",
          "SSD Dell 1,92 ТБ SATA 2,5\" для PowerEdge", "400-BDUO", 69900, 1920, "SSD", "SATA", "2.5",
          "Серверный SSD Dell с салазками. Подходит для системных томов и баз данных среднего размера."),
    _disk("ssd-dell-960-sas", "dell-server-drives", "Dell диски для PowerEdge", "dell",
          "SSD Dell 960 ГБ SAS 12 Гбит/с 2,5\" для PowerEdge", "400-BEGJ", 64900, 960, "SSD", "SAS", "2.5",
          "SAS SSD смешанной нагрузки. Нужен RAID-контроллер с поддержкой SAS."),
    _disk("hdd-dell-2400-sas", "dell-server-drives", "Dell диски для PowerEdge", "dell",
          "Жёсткий диск Dell 2,4 ТБ SAS 10K 2,5\" для PowerEdge", "401-ABHQ", 42900, 2400, "HDD", "SAS", "2.5",
          "Жёсткий диск 10 000 об/мин для файловых хранилищ. Салазки Dell в комплекте."),
    _disk("hdd-dell-8000-sas", "dell-server-drives", "Dell диски для PowerEdge", "dell",
          "Жёсткий диск Dell 8 ТБ SAS 7,2K 3,5\" для PowerEdge", "401-ABHX", 54900, 8000, "HDD", "SAS", "3.5",
          "Диск 3,5 дюйма для архивов и резервных копий. Устанавливается только в шасси с отсеками 3,5 дюйма."),
    _disk("ssd-hpe-960-sata", "hpe-server-drives", "HPE диски для ProLiant", "hpe",
          "SSD HPE 960 ГБ SATA 2,5\" SFF", "P18424-B21", 44900, 960, "SSD", "SATA", "2.5",
          "Серверный SSD HPE в салазках Smart Carrier. Для ProLiant Gen10 и Gen11."),
    _disk("ssd-hpe-1920-sata", "hpe-server-drives", "HPE диски для ProLiant", "hpe",
          "SSD HPE 1,92 ТБ SATA 2,5\" SFF", "P18426-B21", 78900, 1920, "SSD", "SATA", "2.5",
          "Серверный SSD HPE в салазках Smart Carrier. Интенсивное чтение."),
    _disk("ssd-hpe-960-sas", "hpe-server-drives", "HPE диски для ProLiant", "hpe",
          "SSD HPE 960 ГБ SAS 12 Гбит/с 2,5\" SFF", "P40506-B21", 69900, 960, "SSD", "SAS", "2.5",
          "SAS SSD HPE смешанной нагрузки. Нужен контроллер с поддержкой SAS."),
    _disk("hdd-hpe-2400-sas", "hpe-server-drives", "HPE диски для ProLiant", "hpe",
          "Жёсткий диск HPE 2,4 ТБ SAS 10K 2,5\" SFF", "881457-B21", 47900, 2400, "HDD", "SAS", "2.5",
          "Жёсткий диск HPE 10 000 об/мин в салазках Smart Carrier. Для файловых хранилищ и резервных копий."),
    _disk("ssd-hpe-3840-nvme", "hpe-server-drives", "HPE диски для ProLiant", "hpe",
          "SSD HPE 3,84 ТБ NVMe 2,5\" SFF", "P47843-B21", 149900, 3840, "SSD", "NVMe", "2.5",
          "NVMe SSD для высоконагруженных баз данных. Нужна NVMe-корзина в сервере."),
    _disk("ssd-samsung-pm893-960", "samsung-pm893", "Samsung PM893", "samsung",
          "SSD Samsung PM893 960 ГБ SATA 2,5\"", "MZ7L3960HCJR-00A07", 18900, 960, "SSD", "SATA", "2.5",
          "Серверный SSD Samsung без салазок. Совместимость с конкретными серверами магазином не проверялась."),
    _disk("hdd-seagate-exos-8tb", "seagate-exos", "Seagate Exos", "seagate",
          "Жёсткий диск Seagate Exos 7E10 8 ТБ SATA 3,5\"", "ST8000NM000A", 21900, 8000, "HDD", "SATA", "3.5",
          "Корпоративный диск 3,5 дюйма для хранилищ. Поставляется без салазок."),
    _disk("ssd-yadro-960", "yadro-drives", "YADRO диски", "yadro",
          "SSD YADRO 960 ГБ SATA 2,5\" для VEGMAN", "YS-SATA-960-RI", 41900, 960, "SSD", "SATA", "2.5",
          "SSD из списка совместимости VEGMAN с салазками YADRO. Гарантия производителя сервера."),
]


def _svc(pid, fam, fname, vendor, title, mpn, price, months, model, desc):
    return dict(id=pid, family_id=fam, family_name=fname, vendor_id=vendor, category_id="server-services", title=title,
                mpn=mpn, price=price, price_mode="fixed", specs={}, attributes=dict(service_level="NBD"),
                description=desc, license=dict(type="subscription", metric="server", quantity=1,
                                               duration_months=months, edition=""), service_for=model)


SERVICES = [
    _svc("svc-dell-ps-r740-3y", "dell-prosupport", "Dell ProSupport", "dell",
         "Dell ProSupport 3 года, выезд на следующий рабочий день, для PowerEdge R740", "PS-R740-3Y-NBD", 89900, 36,
         R740, "Расширенная гарантия Dell с выездом инженера на следующий рабочий день. Привязывается к сервисному "
               "номеру сервера."),
    _svc("svc-dell-ps-r750-3y", "dell-prosupport", "Dell ProSupport", "dell",
         "Dell ProSupport 3 года, выезд на следующий рабочий день, для PowerEdge R750", "PS-R750-3Y-NBD", 109900, 36,
         R750, "Расширенная гарантия Dell для R750. Привязывается к сервисному номеру сервера."),
    _svc("svc-hpe-tc-g10-3y", "hpe-tech-care", "HPE Tech Care", "hpe",
         "HPE Tech Care Essential 3 года для ProLiant DL380 Gen10", "H70N0E", 94900, 36, G10,
         "Сервисный пакет HPE: замена компонентов на следующий рабочий день и доступ к специалистам. Только для "
         "DL380 Gen10."),
    _svc("svc-hpe-tc-g11-5y", "hpe-tech-care", "HPE Tech Care", "hpe",
         "HPE Tech Care Essential 5 лет для ProLiant DL380 Gen11", "H72Y3E", 189900, 60, G11,
         "Сервисный пакет HPE на пять лет для DL380 Gen11. Замена компонентов на следующий рабочий день."),
    _svc("svc-yadro-3y", "yadro-support", "YADRO Сервис", "yadro",
         "YADRO Сервис 3 года, выезд на следующий рабочий день, для VEGMAN R220", "YS-R220-3Y-NBD", 129900, 36, VEG,
         "Сервисная поддержка YADRO на территории России. Выезд инженера на следующий рабочий день."),
]


def _ups(pid, fam, fname, vendor, title, mpn, price, watts, units, desc):
    return dict(id=pid, family_id=fam, family_name=fname, vendor_id=vendor, category_id="ups", title=title, mpn=mpn,
                price=price, price_mode="fixed", specs=dict(power_w=watts, form_factor_u=units),
                attributes=dict(topology="online" if watts >= 2000 else "line-interactive", mount="стойка"),
                description=desc)


UPS = [
    _ups("ups-ippon-rt-3000", "ippon-innova-rt-ii", "Ippon Innova RT II", "ippon",
         "ИБП Ippon Innova RT II 3000, 2700 Вт, 2U", "INNOVA-RT-II-3000", 98900, 2700, 2,
         "Онлайн-ИБП с двойным преобразованием для серверной стойки. Стойка или башня, сменные батареи."),
    _ups("ups-ippon-rt-6000", "ippon-innova-rt-ii", "Ippon Innova RT II", "ippon",
         "ИБП Ippon Innova RT II 6000, 5400 Вт, 3U", "INNOVA-RT-II-6000", 189900, 5400, 3,
         "Онлайн-ИБП для двух-трёх серверов. Подключение внешних батарейных модулей."),
    _ups("ups-ippon-spp-2200", "ippon-smart-power-pro-ii", "Ippon Smart Power Pro II", "ippon",
         "ИБП Ippon Smart Power Pro II 2200, 1200 Вт", "SPP-II-2200", 38900, 1200, 2,
         "Линейно-интерактивный ИБП для коммутаторов и небольших стоек. Время автономии до 10 минут на половине "
         "нагрузки."),
    _ups("ups-eaton-9sx-3000", "eaton-9sx", "Eaton 9SX", "eaton",
         "ИБП Eaton 9SX 3000i RT3U, 2700 Вт", "9SX3KIRT", 159900, 2700, 3,
         "Онлайн-ИБП для серверных стоек с ЖК-панелью. Цена указана с батареями."),
]


def _lap(pid, fam, fname, vendor, title, mpn, price, ram, ssd, screen, cores, cpu, os_, desc, origin_reestr=False):
    return dict(id=pid, family_id=fam, family_name=fname, vendor_id=vendor, category_id="laptops", title=title, mpn=mpn,
                price=price, price_mode="fixed", specs=dict(ram_gb=ram, storage_gb=ssd, screen_inch=screen,
                                                            cpu_cores=cores),
                attributes=dict(cpu=cpu, os=os_), description=desc, model_id=fam)


LAPTOPS = [
    _lap("nb-e14g5-i5-16-512", "lenovo-thinkpad-e14-gen5", "Lenovo ThinkPad E14 Gen 5", "lenovo",
         "Ноутбук Lenovo ThinkPad E14 Gen 5, 14\", Core i5-1335U, 16 ГБ, SSD 512 ГБ, без ОС", "21JK0083RT", 84900, 16,
         512, 14.0, 10, "Intel Core i5-1335U", "без ОС",
         "Офисный ноутбук 14 дюймов в металлическом корпусе. Поставляется без операционной системы."),
    _lap("nb-e14g5-i7-16-1t", "lenovo-thinkpad-e14-gen5", "Lenovo ThinkPad E14 Gen 5", "lenovo",
         "Ноутбук Lenovo ThinkPad E14 Gen 5, 14\", Core i7-1355U, 16 ГБ, SSD 1 ТБ, с предустановленной ОС", "21JK00C5RT",
         112900, 16, 1000, 14.0, 10, "Intel Core i7-1355U", "предустановлена",
         "Старшая конфигурация E14 с предустановленной операционной системой. Подходит для аналитиков и бухгалтерии."),
    _lap("nb-t14g4-i5-16-512", "lenovo-thinkpad-t14-gen4", "Lenovo ThinkPad T14 Gen 4", "lenovo",
         "Ноутбук Lenovo ThinkPad T14 Gen 4, 14\", Core i5-1345U, 16 ГБ, SSD 512 ГБ, с предустановленной ОС", "21HD0053RT",
         129900, 16, 512, 14.0, 10, "Intel Core i5-1345U", "предустановлена",
         "Бизнес-ноутбук с vPro для корпоративного управления. Операционная система предустановлена."),
    _lap("nb-t14g4-i7-32-1t", "lenovo-thinkpad-t14-gen4", "Lenovo ThinkPad T14 Gen 4", "lenovo",
         "Ноутбук Lenovo ThinkPad T14 Gen 4, 14\", Core i7-1365U, 32 ГБ, SSD 1 ТБ, с предустановленной ОС", "21HD0097RT",
         164900, 32, 1000, 14.0, 10, "Intel Core i7-1365U", "предустановлена",
         "Старшая конфигурация T14 для разработчиков и руководителей. 32 ГБ памяти, операционная система предустановлена."),
    _lap("nb-pb450g10-i5-8-256", "hp-probook-450-g10", "HP ProBook 450 G10", "hp",
         "Ноутбук HP ProBook 450 G10, 15,6\", Core i5-1335U, 8 ГБ, SSD 256 ГБ, без ОС", "816P8EA", 69900, 8, 256,
         15.6, 10, "Intel Core i5-1335U", "без ОС",
         "Недорогой ноутбук 15,6 дюйма с цифровым блоком клавиатуры. Поставляется без операционной системы."),
    _lap("nb-pb450g10-i5-16-512", "hp-probook-450-g10", "HP ProBook 450 G10", "hp",
         "Ноутбук HP ProBook 450 G10, 15,6\", Core i5-1335U, 16 ГБ, SSD 512 ГБ, с предустановленной ОС", "85B04EA", 94900, 16,
         512, 15.6, 10, "Intel Core i5-1335U", "предустановлена",
         "Ноутбук 15,6 дюйма для офиса с предустановленной операционной системой. Цифровой блок клавиатуры."),
    _lap("nb-lat5540-i5-16-512", "dell-latitude-5540", "Dell Latitude 5540", "dell",
         "Ноутбук Dell Latitude 5540, 15,6\", Core i5-1345U, 16 ГБ, SSD 512 ГБ, с предустановленной ОС", "N008L554015RU", 109900,
         16, 512, 15.6, 10, "Intel Core i5-1345U", "предустановлена",
         "Корпоративный ноутбук 15,6 дюйма с vPro. Операционная система предустановлена."),
    _lap("nb-lat5540-i7-32-1t", "dell-latitude-5540", "Dell Latitude 5540", "dell",
         "Ноутбук Dell Latitude 5540, 15,6\", Core i7-1365U, 32 ГБ, SSD 1 ТБ, без ОС", "N012L554015RU", 139900, 32, 1000,
         15.6, 10, "Intel Core i7-1365U", "без ОС",
         "Старшая конфигурация Latitude без операционной системы. 32 ГБ памяти, SSD 1 ТБ."),
    _lap("nb-mbd16-i5-16-512", "huawei-matebook-d16", "Huawei MateBook D 16", "huawei",
         "Ноутбук Huawei MateBook D 16, 16\", Core i5-12450H, 16 ГБ, SSD 512 ГБ, с предустановленной ОС", "53013WXF", 74900,
         16, 512, 16.0, 8, "Intel Core i5-12450H", "предустановлена",
         "Ноутбук 16 дюймов в металлическом корпусе. Предустановлена домашняя редакция ОС; для работы в домене нужна профессиональная."),
    _lap("nb-ns685-i5-8-256", "aquarius-cmp-ns685", "Аквариус Cmp NS685", "aquarius",
         "Ноутбук Аквариус Cmp NS685U R11, 15,6\", Core i5-1135G7, 8 ГБ, SSD 256 ГБ, без ОС", "NS685U-R11-8-256",
         79900, 8, 256, 15.6, 4, "Intel Core i5-1135G7", "без ОС",
         "Российский ноутбук из реестра промышленной продукции. Поставляется без ОС, совместим с Astra Linux и "
         "РЕД ОС."),
    _lap("nb-ns685-i5-16-512", "aquarius-cmp-ns685", "Аквариус Cmp NS685", "aquarius",
         "Ноутбук Аквариус Cmp NS685U R11, 15,6\", Core i5-1135G7, 16 ГБ, SSD 512 ГБ, без ОС", "NS685U-R11-16-512",
         92900, 16, 512, 15.6, 4, "Intel Core i5-1135G7", "без ОС",
         "Российский ноутбук из реестра промышленной продукции, 16 ГБ памяти. Поставляется без ОС."),
]


def _mon(pid, fam, fname, vendor, title, mpn, price, screen, hz, res, panel, desc):
    return dict(id=pid, family_id=fam, family_name=fname, vendor_id=vendor, category_id="monitors", title=title,
                mpn=mpn, price=price, price_mode="fixed", specs=dict(screen_inch=screen, refresh_hz=hz),
                attributes=dict(resolution=res, panel=panel), description=desc)


MONITORS = [
    _mon("mon-dell-p2425h", "dell-p2425h", "Dell P2425H", "dell", "Монитор Dell P2425H, 23,8\", IPS, 1920×1080, 100 Гц",
         "P2425H", 21900, 23.8, 100, "1920×1080", "IPS",
         "Офисный монитор с регулировкой высоты и поворотом. Порт USB-C для подключения ноутбука."),
    _mon("mon-dell-p2725h", "dell-p2725h", "Dell P2725H", "dell", "Монитор Dell P2725H, 27\", IPS, 1920×1080, 100 Гц",
         "P2725H", 27900, 27.0, 100, "1920×1080", "IPS",
         "Монитор 27 дюймов для офиса. Регулировка высоты, поворот в портретный режим."),
    _mon("mon-dell-u2724d", "dell-u2724d", "Dell UltraSharp U2724D", "dell",
         "Монитор Dell UltraSharp U2724D, 27\", IPS, 2560×1440, 120 Гц", "U2724D", 46900, 27.0, 120, "2560×1440", "IPS",
         "Монитор QHD для дизайнеров и аналитиков. Заводская калибровка цвета."),
    _mon("mon-hp-e24g5", "hp-e24-g5", "HP E24 G5", "hp", "Монитор HP E24 G5, 23,8\", IPS, 1920×1080, 75 Гц",
         "6N4E2AA", 18900, 23.8, 75, "1920×1080", "IPS",
         "Офисный монитор HP с тонкой рамкой. Удобен для сборки двух экранов на рабочем месте."),
    _mon("mon-lenovo-l27i", "lenovo-l27i-40", "Lenovo L27i-40", "lenovo",
         "Монитор Lenovo L27i-40, 27\", IPS, 1920×1080, 100 Гц", "67ACKAC4EU", 19900, 27.0, 100, "1920×1080",
         "IPS", "Монитор 27 дюймов в тонком корпусе. Встроенные динамики, наклон экрана."),
    _mon("mon-samsung-g5-27", "samsung-odyssey-g5", "Samsung Odyssey G5", "samsung",
         "Монитор Samsung Odyssey G5, 27\", VA, 2560×1440, 165 Гц", "LC27G55TQBIXCI", 29900, 27.0, 165, "2560×1440",
         "VA", "Изогнутый монитор с частотой 165 Гц. Подходит для работы с графикой и видео."),
    _mon("mon-huawei-se24", "huawei-mateview-se", "Huawei MateView SE", "huawei",
         "Монитор Huawei MateView SE, 23,8\", IPS, 1920×1080, 75 Гц", "SSN-24BZ", 15900, 23.8, 75, "1920×1080", "IPS",
         "Бюджетный монитор для офиса. Наклон и регулировка высоты."),
]


def _sw(pid, fam, fname, vendor, title, mpn, price, ports, gbps, poe_w, desc):
    specs = dict(ports_count=ports, net_gbps=gbps, form_factor_u=1)
    if poe_w:
        specs["power_w"] = poe_w
    return dict(id=pid, family_id=fam, family_name=fname, vendor_id=vendor, category_id="switches", title=title,
                mpn=mpn, price=price, price_mode="fixed", specs=specs,
                attributes=dict(poe=bool(poe_w), layer="L2+" if gbps >= 10 else "L2"), description=desc)


SWITCHES = [
    _sw("sw-eltex-mes2324b", "eltex-mes2300", "Элтекс MES2300", "eltex",
        "Коммутатор Элтекс MES2324B, 24× 1 Гбит/с, 4× 10G SFP+", "MES2324B", 89900, 24, 10, 0,
        "Управляемый коммутатор доступа L2+ российского производства. Четыре аплинка 10 Гбит/с."),
    _sw("sw-eltex-mes2324p", "eltex-mes2300", "Элтекс MES2300", "eltex",
        "Коммутатор Элтекс MES2324P, 24× 1 Гбит/с PoE+, 4× 10G SFP+, 410 Вт", "MES2324P_AC", 129900, 24, 10, 410,
        "Коммутатор PoE+ для IP-телефонов и точек доступа. Бюджет PoE 410 Вт."),
    _sw("sw-eltex-mes2348b", "eltex-mes2300", "Элтекс MES2300", "eltex",
        "Коммутатор Элтекс MES2348B, 48× 1 Гбит/с, 4× 10G SFP+", "MES2348B", 139900, 48, 10, 0,
        "Коммутатор доступа на 48 портов российского производства. Четыре аплинка 10 Гбит/с."),
    _sw("sw-huawei-s5735-24", "huawei-s5735", "Huawei S5735", "huawei",
        "Коммутатор Huawei S5735-L24T4S-A1, 24× 1 Гбит/с, 4× 10G SFP+", "S5735-L24T4S-A1", 84900, 24, 10, 0,
        "Управляемый коммутатор доступа Huawei. Стекирование до девяти устройств."),
    _sw("sw-huawei-s5735-48p", "huawei-s5735", "Huawei S5735", "huawei",
        "Коммутатор Huawei S5735-L48P4X-A1, 48× 1 Гбит/с PoE+, 4× 10G SFP+, 740 Вт", "S5735-L48P4X-A1", 219900, 48,
        10, 740, "Коммутатор PoE+ на 48 портов. Бюджет PoE 740 Вт."),
    _sw("sw-aruba-2930f-24", "hpe-aruba-2930f", "HPE Aruba 2930F", "hpe",
        "Коммутатор HPE Aruba 2930F 24G 4SFP+, 24× 1 Гбит/с, 4× 10G SFP+", "JL253A", 149900, 24, 10, 0,
        "Коммутатор доступа уровня L3 lite. Управление через Aruba Central."),
    _sw("sw-aruba-cx6100-48p", "hpe-aruba-cx6100", "HPE Aruba CX 6100", "hpe",
        "Коммутатор HPE Aruba CX 6100 48G Class4 PoE 4SFP+, 370 Вт", "JL675A", 259900, 48, 10, 370,
        "Коммутатор PoE на 48 портов с ОС AOS-CX. Бюджет PoE 370 Вт."),
    _sw("sw-dlink-dgs1210-28", "dlink-dgs-1210", "D-Link DGS-1210", "dlink",
        "Коммутатор D-Link DGS-1210-28, 24× 1 Гбит/с, 4× SFP", "DGS-1210-28/F3A", 24900, 24, 1, 0,
        "Простой настраиваемый коммутатор для небольшого офиса. Аплинки 1 Гбит/с."),
]

HARDWARE = SERVERS + MEMORY + DISKS + SERVICES + UPS + LAPTOPS + MONITORS + SWITCHES
# Цена по запросу у части комплектующих и сети (поставка под проект).
for _p in HARDWARE:
    if _p["id"] in ("sw-aruba-cx6100-48p", "ssd-hpe-3840-nvme", "ups-eaton-9sx-3000"):
        _p["price_mode"], _p["price"] = "quote_required", None

MARGIN_RANGE = {"server-systems": (0.06, 0.10), "server-memory": (0.14, 0.22), "server-storage": (0.12, 0.20),
                "server-services": (0.25, 0.36), "ups": (0.12, 0.18), "laptops": (0.05, 0.09),
                "monitors": (0.08, 0.12), "switches": (0.10, 0.16), "os-server": (0.14, 0.18)}

# ------------------------------------------------------------------ совместимость компонентов
# (product_id | family:<id>, target_model, state, condition)
COMPAT = [
    ("mem-dell-16g-d4", R740, "conditional", "Работает на 2933 МТ/с с Xeon Scalable второго поколения"),
    ("mem-dell-16g-d4", R750, "verified", ""),
    ("mem-dell-32g-d4", R740, "conditional", "Работает на 2933 МТ/с с Xeon Scalable второго поколения"),
    ("mem-dell-32g-d4", R750, "verified", ""),
    ("mem-dell-64g-d4", R740, "conditional", "Не смешивать с LRDIMM; работает на 2933 МТ/с"),
    ("mem-dell-64g-d4", R750, "verified", ""),
    ("mem-dell-32g-d4-2933", R740, "verified", ""),
    ("mem-dell-32g-d4", G10, "incompatible", "Модули Dell без прошивки HPE Smart Memory"),
    ("mem-hpe-16g-d4", G10, "verified", ""),
    ("mem-hpe-32g-d4", G10, "verified", ""),
    ("mem-hpe-64g-d4", G10, "verified", ""),
    ("mem-hpe-16g-d4", G11, "incompatible", "DDR4 не устанавливается в слоты DDR5"),
    ("mem-hpe-32g-d4", G11, "incompatible", "DDR4 не устанавливается в слоты DDR5"),
    ("mem-hpe-64g-d4", G11, "incompatible", "DDR4 не устанавливается в слоты DDR5"),
    ("mem-hpe-16g-d5", G11, "verified", ""),
    ("mem-hpe-32g-d5", G11, "verified", ""),
    ("mem-hpe-64g-d5", G11, "verified", ""),
    ("mem-hpe-32g-d5", G10, "incompatible", "DDR5 не устанавливается в слоты DDR4"),
    ("mem-hpe-32g-d5", R740, "incompatible", "DDR5 не устанавливается в слоты DDR4"),
    ("mem-hpe-32g-d5", R750, "incompatible", "DDR5 не устанавливается в слоты DDR4"),
    ("mem-kingston-32g-d4", R740, "conditional", "Подтверждено Kingston, не Dell; гарантия Dell на модуль не "
                                                 "распространяется"),
    ("mem-kingston-32g-d4", R750, "conditional", "Подтверждено Kingston, не Dell; гарантия Dell на модуль не "
                                                 "распространяется"),
    ("mem-kingston-32g-d5", G11, "conditional", "Подтверждено Kingston, не HPE; без функций HPE Smart Memory"),
    ("mem-kingston-32g-d5", R740, "incompatible", "DDR5 не устанавливается в слоты DDR4"),
    ("mem-kingston-32g-d4", G11, "incompatible", "DDR4 не устанавливается в слоты DDR5"),
    ("mem-yadro-32g-d4", VEG, "verified", ""),
    ("ssd-dell-960-sata", R740, "verified", ""),
    ("ssd-dell-960-sata", R750, "verified", ""),
    ("ssd-dell-1920-sata", R740, "verified", ""),
    ("ssd-dell-1920-sata", R750, "verified", ""),
    ("ssd-dell-960-sas", R740, "verified", ""),
    ("ssd-dell-960-sas", R750, "verified", ""),
    ("hdd-dell-2400-sas", R740, "verified", ""),
    ("hdd-dell-2400-sas", R750, "conditional", "Только в шасси с отсеками 2,5\" SAS/SATA"),
    ("hdd-dell-8000-sas", R740, "conditional", "Только в шасси с отсеками 3,5\""),
    ("hdd-dell-8000-sas", R750, "conditional", "Только в шасси с отсеками 3,5\""),
    ("ssd-dell-960-sata", G10, "incompatible", "Салазки и прошивка Dell, в HPE Smart Carrier не устанавливается"),
    ("ssd-hpe-960-sata", G10, "verified", ""),
    ("ssd-hpe-960-sata", G11, "verified", ""),
    ("ssd-hpe-1920-sata", G10, "verified", ""),
    ("ssd-hpe-1920-sata", G11, "verified", ""),
    ("ssd-hpe-960-sas", G10, "conditional", "Нужен контроллер SAS: штатный S100i работает только с SATA"),
    ("ssd-hpe-960-sas", G11, "verified", ""),
    ("hdd-hpe-2400-sas", G10, "verified", ""),
    ("hdd-hpe-2400-sas", G11, "verified", ""),
    ("ssd-hpe-3840-nvme", G11, "conditional", "Нужна NVMe-корзина в сервере"),
    ("ssd-hpe-960-sata", R740, "incompatible", "Салазки HPE Smart Carrier не подходят к отсекам Dell"),
    ("hdd-seagate-exos-8tb", G11, "incompatible", "В DL380 Gen11 этой конфигурации только отсеки 2,5\""),
    ("ssd-yadro-960", VEG, "verified", ""),
    ("svc-dell-ps-r740-3y", R740, "verified", ""),
    ("svc-dell-ps-r750-3y", R750, "verified", ""),
    ("svc-hpe-tc-g10-3y", G10, "verified", ""),
    ("svc-hpe-tc-g11-5y", G11, "verified", ""),
    ("svc-yadro-3y", VEG, "verified", ""),
    ("svc-dell-ps-r740-3y", R750, "incompatible", "Сервисный пакет привязан к модели R740"),
    ("svc-hpe-tc-g10-3y", G11, "incompatible", "Сервисный пакет привязан к модели DL380 Gen10"),
]
COMPAT_SOURCE = {"dell": ("vendor_documentation", "dell"), "hpe": ("vendor_documentation", "hpe"),
                 "yadro": ("vendor_documentation", "yadro"), "kingston": ("vendor_documentation", "kingston"),
                 "seagate": ("expert", None)}

# ------------------------------------------------------------------ правила лицензирования
LICENSE_RULES = [
    dict(id="lr-ws25-cal", vendor_id="microsoft", anchor_family="windows-server-2025", anchor_attrs={},
         requires_category="os-server", requires_attrs={"edition": "User CAL|Device CAL", "version": ">=anchor.version"},
         **{"class": "conditional_required"}, conditions=["access_license_required", "no_existing_equivalent_license"],
         quantity_strategy="per_user",
         reason="Каждому пользователю или устройству, которые обращаются к Windows Server 2025, нужна CAL версии "
                "2025 или новее, если действующих CAL нет.",
         source="vendor_documentation", source_url=MS_CAL_URL),
    dict(id="lr-ws25-rds", vendor_id="microsoft", anchor_family="windows-server-2025", anchor_attrs={},
         requires_category="os-server", requires_attrs={"edition": "RDS CAL", "version": ">=anchor.version"},
         **{"class": "conditional_required"},
         conditions=["remote_desktop_used", "access_license_required", "no_existing_equivalent_license"],
         quantity_strategy="per_user",
         reason="Для подключения к удалённому рабочему столу Windows Server 2025 нужна RDS CAL 2025 в дополнение к "
                "обычной CAL; RDS CAL прежних версий не подходят.",
         source="vendor_documentation", source_url=MS_RDS_URL),
    dict(id="lr-ws22-cal", vendor_id="microsoft", anchor_family="windows-server-2022", anchor_attrs={},
         requires_category="os-server", requires_attrs={"edition": "User CAL|Device CAL", "version": ">=anchor.version"},
         **{"class": "conditional_required"}, conditions=["access_license_required", "no_existing_equivalent_license"],
         quantity_strategy="per_user",
         reason="Каждому пользователю или устройству, которые обращаются к Windows Server 2022, нужна CAL версии "
                "2022 или новее.",
         source="vendor_documentation", source_url=MS_CAL_URL),
    dict(id="lr-ws22-rds", vendor_id="microsoft", anchor_family="windows-server-2022", anchor_attrs={},
         requires_category="os-server", requires_attrs={"edition": "RDS CAL", "version": ">=anchor.version"},
         **{"class": "conditional_required"},
         conditions=["remote_desktop_used", "access_license_required", "no_existing_equivalent_license"],
         quantity_strategy="per_user",
         reason="Для удалённого рабочего стола Windows Server 2022 нужна RDS CAL 2022 или новее.",
         source="vendor_documentation", source_url=MS_RDS_URL),
    dict(id="lr-sql22-cal", vendor_id="microsoft", anchor_family="sql-server-2022", anchor_product="sql22-stdsrv",
         anchor_attrs={"edition": "Standard"}, requires_category="dbms",
         requires_attrs={"edition": "CAL", "version": ">=anchor.version"}, **{"class": "conditional_required"},
         conditions=["access_license_required", "no_existing_equivalent_license"], quantity_strategy="per_user",
         reason="В модели Server + CAL каждому пользователю SQL Server нужна SQL CAL; при лицензировании по ядрам "
                "CAL не нужны.",
         source="vendor_documentation", source_url=MS_SQL_URL),
    dict(id="lr-1c-server-clients", vendor_id="1c", anchor_family="1c-server", anchor_attrs={},
         requires_category="1c-licenses", requires_attrs={}, requires_family="1c-client-license",
         **{"class": "conditional_required"}, conditions=["access_license_required", "no_existing_equivalent_license"],
         quantity_strategy="per_user",
         reason="Сервер 1С не включает клиентские лицензии: на каждое одновременное подключение нужна клиентская "
                "лицензия 1С.",
         source="vendor_documentation", source_url=ONEC_URL),
]

# ------------------------------------------------------------------ реестр атрибутов
_GB = ["гб", "gb", "г", "гиг", "гигабайт", "гигабайта"]
ATTR = {
    "ram_gb": dict(key="ram_gb", label="Память", unit="ГБ", type="number", facet=True, unit_aliases=_GB),
    "storage_gb": dict(key="storage_gb", label="Накопитель", unit="ГБ", type="number", facet=True, unit_aliases=_GB,
                       unit_convert={"тб": 1000, "tb": 1000, "терабайт": 1000}),
    "screen_inch": dict(key="screen_inch", label="Диагональ", unit="″", type="number", facet=True,
                        unit_aliases=["дюйм", "дюйма", "дюймов", "\"", "″", "in", "inch"]),
    "refresh_hz": dict(key="refresh_hz", label="Частота обновления", unit="Гц", type="number", facet=True,
                       unit_aliases=["гц", "hz", "герц"]),
    "ports_count": dict(key="ports_count", label="Порты", unit="портов", type="number", facet=True,
                        unit_aliases=["порт", "порта", "портов", "port", "ports", "x", "×"]),
    "net_gbps": dict(key="net_gbps", label="Скорость аплинка", unit="Гбит/с", type="number", facet=True,
                     unit_aliases=["гбит/с", "гбит", "gbps", "gbit", "g", "ge"]),
    "power_w": dict(key="power_w", label="Мощность", unit="Вт", type="number", facet=True,
                    unit_aliases=["вт", "w", "ватт"], unit_convert={"квт": 1000, "kw": 1000}),
    "form_factor_u": dict(key="form_factor_u", label="Высота в стойке", unit="U", type="number", facet=True,
                          unit_aliases=["u", "юнит", "юнита", "юнитов"]),
    "cpu_cores": dict(key="cpu_cores", label="Ядер процессора", unit="ядер", type="number", facet=False,
                      unit_aliases=["ядро", "ядра", "ядер", "core", "cores"]),
    "memory_type": dict(key="memory_type", label="Тип памяти", unit="", type="enum", facet=True, unit_aliases=[],
                        values=["DDR4", "DDR5"]),
    "drive_type": dict(key="drive_type", label="Тип накопителя", unit="", type="enum", facet=True, unit_aliases=[],
                       values=["SSD", "HDD"]),
    "interface": dict(key="interface", label="Интерфейс", unit="", type="enum", facet=True, unit_aliases=[],
                      values=["SATA", "SAS", "NVMe"]),
    "drive_form_factor": dict(key="drive_form_factor", label="Форм-фактор", unit="″", type="enum", facet=True,
                              unit_aliases=[], values=["2.5", "3.5"]),
    "os": dict(key="os", label="Операционная система", unit="", type="enum", facet=True, unit_aliases=[],
               values=["без ОС", "предустановлена"]),
    "poe": dict(key="poe", label="PoE", unit="", type="bool", facet=True, unit_aliases=[]),
    "version": dict(key="version", label="Версия", unit="", type="enum", facet=True, unit_aliases=[],
                    values=["2022", "2025"]),
}
ATTR_BY_CATEGORY = {
    "server-systems": ["form_factor_u", "cpu_cores", "ram_gb", "storage_gb", "power_w", "memory_type"],
    "server-memory": ["ram_gb", "memory_type"],
    "server-storage": ["storage_gb", "drive_type", "interface", "drive_form_factor"],
    "server-services": [],
    "laptops": ["ram_gb", "storage_gb", "screen_inch", "cpu_cores", "os"],
    "monitors": ["screen_inch", "refresh_hz"],
    "switches": ["ports_count", "net_gbps", "power_w", "form_factor_u", "poe"],
    "ups": ["power_w", "form_factor_u"],
    "os-server": ["version"],
    "dbms": ["version"],
}

# ------------------------------------------------------------------ шаблоны решений
SOLUTION_TEMPLATES = [
    dict(id="tmpl-workplace", title="Рабочее место сотрудника",
         triggers=["рабочее место", "рабочих мест", "новый сотрудник", "нового сотрудника", "оснастить офис"],
         approved=True, slots=[
             dict(id="laptop", label="Ноутбук", category_id="laptops", query="ноутбук 16 гб",
                  constraints={"ram_gb": {"op": "gte", "value": 16}}, quantity_strategy="per_seat"),
             dict(id="os", label="Операционная система (если ноутбук без ОС)", category_id="os-desktop",
                  query="windows 11 pro", constraints={"if_slot_attr": {"slot": "laptop", "key": "os",
                                                                       "value": "без ОС"}},
                  quantity_strategy="per_seat"),
             dict(id="office", label="Офисный пакет", category_id="office-suite",
                  query="microsoft 365 business standard", constraints={}, quantity_strategy="per_seat"),
             dict(id="security", label="Защита рабочего места", category_id="antivirus-business",
                  query="kaspersky endpoint security", constraints={}, quantity_strategy="per_seat"),
         ]),
    dict(id="tmpl-1c-server", title="Сервер под 1С",
         triggers=["сервер под 1с", "сервер для 1с", "сервер 1с на"], approved=True, slots=[
             dict(id="server", label="Сервер", category_id="server-systems", query="сервер 2u",
                  constraints={"ram_gb": {"op": "gte", "value": 128}}, quantity_strategy="one"),
             dict(id="warranty", label="Гарантия", category_id="server-services", query="гарантия на сервер",
                  constraints={"compatible_with": "slot:server"}, quantity_strategy="one"),
             dict(id="os", label="Серверная ОС", category_id="os-server", query="windows server 2025 standard",
                  constraints={"version": {"op": "eq", "value": "2025"}}, quantity_strategy="per_core"),
             dict(id="cal", label="Лицензии доступа к серверу", category_id="os-server", query="windows server cal 2025",
                  constraints={"version": {"op": "gte", "value": "2025"}}, quantity_strategy="per_user"),
             dict(id="1c-server", label="Сервер 1С:Предприятие", category_id="1c-licenses", query="сервер 1с",
                  constraints={}, quantity_strategy="one"),
             dict(id="1c-clients", label="Клиентские лицензии 1С", category_id="1c-licenses",
                  query="клиентская лицензия 1с", constraints={}, quantity_strategy="per_user"),
             dict(id="dbms", label="СУБД", category_id="dbms", query="postgres pro", constraints={},
                  quantity_strategy="one"),
         ]),
]
