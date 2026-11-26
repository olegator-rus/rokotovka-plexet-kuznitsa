# Разметка ожиданий для прогона реальных и собственных запросов (scripts/eval/real_queries.py).
# want — что считается попаданием в топ-3: id семейства или "cat:<категория>"; пусто — только смотреть глазами.
# absent — точного товара в каталоге нет: честное «нет» или ближайшая категория из want.
WS = ["windows-server-2025", "windows-server-2022"]
WSCAL = ["windows-server-cal-2025", "windows-server-cal"]
RDS = ["rds-cal-2025"]
KASP = ["cat:antivirus-business", "kaspersky-internet-security", "kaspersky-standard", "kaspersky-plus", "kaspersky-premium"]
WIN = ["windows-11-pro", "windows-11-home", "windows-10-pro"]
SRV1C = ["cat:server-systems", "1c-server"]
REMOTE = ["anydesk", "assistant"]
RUOS = ["astra-linux-se", "red-os", "alt-os"]
OFFICE = ["office-2021", "office-2024", "microsoft-365-business", "microsoft-365-home"]

L = []
def q(src, text, want=(), absent=False):
    L.append({"src": src, "q": text, "want": list(want), "absent": absent})

# --- реальные: форумы (data/real-queries/forum-batch-1.jsonl) ---
F = "forum"
q(F, "Сервер для 1с предприятия?", SRV1C)
q(F, "Какое количество лицензий CAL необходимо приобретать?", WSCAL + RDS)
q(F, "Как правильно лицензировать 1С на терминальном сервере?", ["1c-client-license", "1c-server"] + RDS)
q(F, "Какие лицензии необходимы для РАБОТЫ MS VDI?", RDS + WS + WSCAL, absent=True)
q(F, "Какую лицензию выбрать для MS Server?", WS)
q(F, "Выбор антивируса для организации?", ["cat:antivirus-business"])
q(F, "Выбор антивируса DrWeb или Nod32?", ["drweb-enterprise-security-suite", "drweb-security-space", "eset-nod32-antivirus", "eset-protect", "eset-smart-security-premium"])
q(F, "Антивирус для оператора персональных данных. Можно ли обойтись без ФСТЭК?", ["cat:antivirus-business"])
q(F, "Какой есть толковй антивирус (не дорого), для небольшой организации (10-15 ПК)?", ["cat:antivirus-business"])
q(F, "Какой можете посоветовать ИБП для серверной?", ["cat:ups"])
q(F, "Какие гипервизоры есть для замены Hyper-V / VMware по импортозамещению?", ["alt-os"])
q(F, "Какое оборудование для серверного шкафа лучше использовать и оборудовать серверную?", ["cat:server-systems", "cat:ups", "cat:switches"])
q(F, "Выбор коммутатора для небольшой организации - L2 или L3?", ["cat:switches"])
q(F, "Какой софт, кроме NUT и Parachute еще есть для ИБП?", ["cat:ups"], absent=True)
q(F, "Какое сетевое оборудование выбрать: Mikrotik или Zyxel?", ["cat:switches"], absent=True)
q(F, "Альтернатива Teamviewer?", REMOTE)
q(F, "Как лицензировать Windows Server в нестандартных ситуациях?", WS + WSCAL)
q(F, "Стоит ли устанавливать Астра линукс и чем его можно заменить?", RUOS)
q(F, "Что использовать взамен TeamViewer напрямую без доступа в интернет?", REMOTE)
q(F, "Чем заменить Teamviewer?", REMOTE)
q(F, "Какая сейчас в РФ ситуация касательно лицензирования ПО от Майкрософт?", [])
q(F, "Какую систему резервного копирования для Windows выбрать?", ["cat:backup"])
q(F, "Лицензирование windows server что покупать?", WS + WSCAL)
q(F, "Как приобрести лицензию Windows для компании?", WIN + WS)
q(F, "Как подобрать сервер для 1с и не только?", SRV1C)
q(F, "teamviewer аналоги", REMOTE)
q(F, "TeamViewer - нормально ли им ещё пользоваться, какие альтернативы?", REMOTE)
q(F, "Есть ли программы удаленного управления, которые поддерживают RemoteApp и работают за NAT?", ["cat:remote-access"] + RDS)
q(F, "Как выбрать новый сервер?", ["cat:server-systems"])
q(F, "Какую версию Windows Server лучше купить для юр. организации?", WS)
q(F, "Какое ПО для удалённой поддержки вы использовали?", ["cat:remote-access"])
q(F, "Какую оперативную память использует Dell для своих серверов?", ["dell-rdimm-ddr4"])
q(F, "Как лучше организовать сервер для 1с?", SRV1C)
q(F, "Какой продукт выбрать для резервного копирования виртуальной инфраструктуры?", ["cat:backup"])
q(F, "Bacula или Bareos или \"что-то еще\". Что выбрать в качестве системы резервного копирования?", ["cat:backup"], absent=True)
q(F, "Где хранить бэкапы всех систем?", ["cat:backup", "cat:server-storage"])
q(F, "Где купить Б/У сервер? Как не попасть в подставу? И какой нужен по конфигурации сервер?", ["cat:server-systems"])
q(F, "Стоит ли покупать лицензию MS Office в интернет-магазинах?", OFFICE)
q(F, "Какой MS Office я должен закупить чтобы задействовать его на терминальном сервере Microsoft Windows Server?", OFFICE)
q(F, "Управление корпоративными лицензиями Windows в текущих условиях?", WIN + WS)
q(F, "Как использовать бесплатное ПО образовательной организации?", [])
q(F, "Лицензирование MSSQL Server 2012", ["sql-server-2022"], absent=True)
q(F, "Лицензирование MS SQL Server для сервера 1С", ["sql-server-2022", "postgres-pro"])
q(F, "Windows Server License, что и как?", WS + WSCAL)
q(F, "Лицензия Microsoft SQL Server Standard 2014 x64 RUS – 1 шт.", ["sql-server-2022"], absent=True)
q(F, "Лицензия Microsoft Windows Server Standard R2 2012 x64 RUS – 1 шт.", WS, absent=True)

# --- реальные: вторая пачка (data/real-queries/batch-2.jsonl) ---
B = "batch2"
q(B, "купить лицензию Windows Server", WS)
q(B, "windows server 2022 лицензия", ["windows-server-2022"])
q(B, "Windows Server CAL", WSCAL)
q(B, "Windows Server Standard", WS)
q(B, "Windows Server Datacenter", WS)
q(B, "Windows Server 2025 50 Devices CAL WW LTU", ["windows-server-cal-2025"])
q(B, "Windows Server 2025 10 Users CAL WW LTU", ["windows-server-cal-2025"])
q(B, "Win Rmt Dsktp Svcs CAL 2019 English MLP AE Device CAL", RDS, absent=True)
q(B, "Win Rmt Dsktp Svcs CAL 2019 English MLP AE User CAL", RDS, absent=True)
q(B, "6VC-03790", RDS, absent=True)
q(B, "6VC-03791", RDS, absent=True)
q(B, "Windows Server CAL 2019 English MLP 5 User CAL", WSCAL, absent=True)
q(B, "R18-05657", WSCAL, absent=True)
q(B, "OS1001Х8617BOX000SR01-PR12", ["astra-linux-se"], absent=True)
q(B, "Astra Linux Special Edition для сервера без ограничения срока", ["astra-linux-se"])
q(B, "Astra Linux Special Edition уровень защищенности Базовый Орел ФСТЭК", ["astra-linux-se"])
q(B, "что нужно к windows server чтобы 20 человек подключались", WSCAL + RDS + WS)
q(B, "cal к sql", ["sql-server-2022"])
q(B, "виндовс сервер", WS)
q(B, "кспрский", KASP)
q(B, "цштвщцы", WIN + WS)
q(B, "офис 2о21", ["office-2021"])
q(B, "винда", WIN)
q(B, "вин серв", WS)
q(B, "каспер", KASP)
q(B, "кис", ["kaspersky-internet-security"])
q(B, "кес", ["kaspersky-endpoint-security"])
q(B, "офис 365", ["microsoft-365-business", "microsoft-365-home"])
q(B, "эмэс скуль", ["sql-server-2022"])
q(B, "астра смоленск", ["astra-linux-se"])
q(B, "рдп кал", RDS)
q(B, "антивирус для офиса на 20 компов", ["cat:antivirus-business"])
q(B, "чем заменить vmware", ["alt-os"])
q(B, "ПО для удалённого доступа к рабочим компам", ["cat:remote-access"])
q(B, "сервер для 1с на 30 пользователей", SRV1C + ["1c-client-license"])
q(B, "кис 10 челов 2 года", ["kaspersky-internet-security"])
q(B, "кис 10 челов2 года", ["kaspersky-internet-security"])
q(B, "на 50 юзеров", [])
q(B, "на 3 пк на год", [])
q(B, "25 устройств", [])
q(B, "5пк", [])
q(B, "память для r740", ["dell-rdimm-ddr4", "kingston-server-premier", "samsung-rdimm"])
q(B, "диски для proliant dl380 gen10", ["hpe-server-drives"])
q(B, "дешёвый антивирус для бизнеса", ["cat:antivirus-business"])
q(B, "до 50 тыс", [])
q(B, "прайс", [])
q(B, "лицензия", [])
q(B, "PER740-", ["dell-poweredge-r740"])
q(B, "P50751-B21", [], absent=True)
q(B, "KL4863RA", ["kaspersky-endpoint-security", "cat:antivirus-business"], absent=True)
q(B, "DG7GMGF0D7", [], absent=True)

# --- свои: опечатки, сленг, раскладка, слитно, артикулы, импортозамещение, ТЗ ---
O = "own"
for text, want in [
    ("виндоус 11 про", ["windows-11-pro"]), ("windovs 11", ["windows-11-pro", "windows-11-home"]),
    ("win11pro", ["windows-11-pro"]), ("win 10 pro", ["windows-10-pro"]), ("виндовс 11 домашняя", ["windows-11-home"]),
    ("windows 11 pro oem", ["windows-11-pro"]), ("ws2022", ["windows-server-2022"]), ("ws 2025 dc", ["windows-server-2025"]),
    ("windows server 2025 standard 16 core", ["windows-server-2025"]), ("cal 2025 user 5 шт", ["windows-server-cal-2025"]),
    ("rds cal 2025 на 10 пользователей", RDS), ("терминальные лицензии", RDS), ("лицензии на удаленный рабочий стол", RDS),
    ("зрщещырщз", ["adobe-photoshop"]), ("фотошоп", ["adobe-photoshop"]), ("фотошоп на год", ["adobe-photoshop"]),
    ("иллюстратор", ["adobe-illustrator"]), ("креатив клауд", ["adobe-creative-cloud"]),
    ("аналог фотошопа", ["affinity-photo", "fotomaster"]), ("автокад", ["autodesk-autocad", "autodesk-autocad-lt"]),
    ("автокад лт", ["autodesk-autocad-lt"]), ("компас 3д", ["kompas-3d"]), ("нанокад", ["nanocad"]),
    ("аналог автокада российский", ["nanocad", "kompas-3d", "renga"]), ("bim российский", ["renga"]),
    ("корел", ["coreldraw-graphics-suite"]), ("акробат", ["adobe-acrobat"]), ("файнридер", ["abbyy-finereader-pdf"]),
    ("пдф редактор", ["cat:office-pdf"]), ("доктор веб", ["drweb-enterprise-security-suite", "drweb-security-space"]),
    ("нод 32", ["eset-nod32-antivirus", "eset-smart-security-premium"]), ("есет", ["eset-nod32-antivirus", "eset-protect", "eset-smart-security-premium"]),
    ("касперский малый офис", ["kaspersky-small-office-security"]), ("ксос на 5 пк", ["kaspersky-small-office-security"]),
    ("kaspersky endpoint 50 узлов", ["kaspersky-endpoint-security"]), ("кес50", ["kaspersky-endpoint-security"]),
    ("kes 100", ["kaspersky-endpoint-security"]), ("касперский для бизнеса на 2 года", ["kaspersky-endpoint-security", "kaspersky-small-office-security"]),
    ("антивирус для дома на 3 устройства", ["cat:antivirus-home"]), ("антивирус на 100 устройств", ["cat:antivirus-business"]),
    ("антивирус для linux серверов", ["kaspersky-endpoint-security", "drweb-enterprise-security-suite", "eset-protect"]),
    ("антивирус для астры", ["kaspersky-endpoint-security", "drweb-enterprise-security-suite"]),
    ("продление касперского", ["cat:antivirus-business", "kaspersky-internet-security", "kaspersky-standard", "kaspersky-plus", "kaspersky-premium"]),
    ("продлить антивирус dr web", ["drweb-enterprise-security-suite", "drweb-security-space"]),
    ("antivirus for business", ["cat:antivirus-business"]),
    ("1с бух проф", ["1c-accounting"]), ("бухия корп", ["1c-accounting"]), ("зуп", ["1c-zup"]), ("ут 11", ["1c-trade-management"]),
    ("1с ка", ["1c-complex-automation"]), ("лицензия 1с на 5 пользователей", ["1c-client-license"]),
    ("серверная лицензия 1с x64", ["1c-server"]), ("итс проф", ["1c-its"]), ("1с розница", ["1c-roznica"]), ("1с8.3", ["1c-client-license", "1c-server"]),
    ("мойофис", ["myoffice-standard"]), ("р7 офис", ["r7-office-professional"]), ("r7 office", ["r7-office-professional"]),
    ("российский офисный пакет", ["myoffice-standard", "r7-office-professional"]), ("замена ms office", ["myoffice-standard", "r7-office-professional"]),
    ("офис2021", ["office-2021"]), ("office 2024 для дома и бизнеса", ["office-2024"]), ("ворд", OFFICE), ("эксель лицензия", OFFICE),
    ("m365 business standard", ["microsoft-365-business"]), ("майкрософт 365 на год", ["microsoft-365-business", "microsoft-365-home"]),
    ("outlook", OFFICE), ("sql server standard 2022", ["sql-server-2022"]), ("скуль сервер ядро", ["sql-server-2022"]),
    ("постгрес про", ["postgres-pro"]), ("postgresql enterprise", ["postgres-pro"]), ("российская субд", ["postgres-pro"]),
    ("ред ос", ["red-os"]), ("redos", ["red-os"]), ("альт линукс", ["alt-os"]), ("российская ос для рабочих станций", RUOS),
    ("ос из реестра", RUOS), ("астра 1.8 воронеж", ["astra-linux-se"]), ("отечественная виртуализация", ["alt-os"]),
    ("vsphere essentials", ["vmware-vsphere"]), ("вмваре", ["vmware-vsphere"]), ("виим", ["veeam-backup"]), ("veeam", ["veeam-backup"]),
    ("кибер бекап", ["cyber-backup"]), ("акронис", ["cyber-backup"]), ("бэкап серверов", ["cat:backup"]),
    ("энидеск", ["anydesk"]), ("тимвьювер", ["teamviewer"]), ("российский аналог teamviewer", ["assistant"]),
    ("зум", ["zoom-workplace"]), ("видеоконференции российские", ["trueconf-server"]), ("трукокнф", ["trueconf-server"]),
    ("яндекс 360", ["yandex-360"]), ("корпоративная почта", ["yandex-360", "microsoft-365-business"]),
    ("пайчарм", ["pycharm"]), ("идея ультимейт", ["intellij-idea"]), ("visual studio professional", ["visual-studio"]),
    ("jetbrains all products", ["jetbrains-all-products"]), ("крипто про", ["cryptopro-csp"]), ("криптопро csp 5", ["cryptopro-csp"]),
    ("эцп программа", ["cryptopro-csp"]), ("dlp система", ["infowatch-traffic-monitor"]), ("сзи от нсд", ["secret-net-studio"]),
    ("сканер уязвимостей", ["maxpatrol-vm"]), ("waf", ["pt-application-firewall"]),
    ("сервер dell r750", ["dell-poweredge-r750"]), ("hpe dl380 gen11", ["hpe-proliant-dl380-gen11"]), ("proliant gen10", ["hpe-proliant-dl380-gen10"]),
    ("отечественный сервер", ["yadro-vegman-r220-g2"]), ("ядро вегман", ["yadro-vegman-r220-g2"]),
    ("сервер 2u 2 процессора 256 гб", ["cat:server-systems"]), ("память ddr5 32gb rdimm", ["hpe-smart-memory-ddr5", "kingston-server-premier"]),
    ("ssd 960 sata для сервера", ["cat:server-storage"]), ("жесткий диск 8тб sas", ["dell-server-drives", "seagate-exos"]),
    ("ибп 3 ква", ["cat:ups"]), ("ибп ippon", ["ippon-innova-rt-ii", "ippon-smart-power-pro-ii"]), ("eaton 9sx", ["eaton-9sx"]),
    ("коммутатор 24 порта poe", ["cat:switches"]), ("eltex mes2300", ["eltex-mes2300"]), ("свитч huawei", ["huawei-s5735"]),
    ("ноутбук для офиса", ["cat:laptops"]), ("thinkpad t14", ["lenovo-thinkpad-t14-gen4"]), ("ноутбук российский", ["aquarius-cmp-ns685"]),
    ("монитор 27 дюймов", ["cat:monitors"]), ("монитор dell 24", ["dell-p2425h"]),
    ("гарантия на сервер dell", ["dell-prosupport"]), ("продление поддержки hpe", ["hpe-tech-care"]),
    ("remote desktop license", RDS),
    ("Лицензия на право использования антивирусного программного обеспечения для защиты 50 рабочих станций и 2 серверов сроком на 12 месяцев",
     ["kaspersky-endpoint-security", "drweb-enterprise-security-suite", "eset-protect", "kaspersky-small-office-security"]),
    ("Поставка неисключительных прав на использование операционной системы общего назначения, включенной в реестр российского ПО, для 30 АРМ", RUOS),
    ("Право использования СУБД отечественного производства", ["postgres-pro"]),
]:
    q(O, text, want)

# --- свои, вторая пачка: железо по характеристикам, бюджет, английский, сравнения, образование, комплекты ---
O2 = "own2"
for text, want in [
    ("сервер 64 гб ddr4", ["cat:server-systems"]), ("сервер на 2 процессора xeon gold", ["cat:server-systems"]),
    ("ssd nvme 3.84 тб", ["hpe-server-drives"]), ("диск sas 2.4 тб 10k", ["dell-server-drives", "hpe-server-drives"]),
    ("память 64гб ddr4 3200", ["dell-rdimm-ddr4", "samsung-rdimm", "hpe-smart-memory-ddr4"]),
    ("монитор 4к", ["dell-u2724d", "cat:monitors"]), ("монитор для офиса 24 дюйма", ["cat:monitors"]),
    ("ноутбук 14 дюймов", ["cat:laptops"]), ("ноутбук 16 гб озу 512 ssd", ["cat:laptops"]),
    ("ибп на 1500 ва", ["cat:ups"]), ("ибп в стойку", ["cat:ups"]), ("коммутатор 48 портов", ["cat:switches"]),
    ("управляемый коммутатор l3", ["cat:switches"]),
    ("антивирус до 5000 рублей", ["cat:antivirus-home", "cat:antivirus-business"]),
    ("недорогой офисный пакет", ["cat:office-suite"]), ("бесплатный антивирус", ["cat:antivirus-home", "cat:antivirus-business"]),
    ("windows server standard 2025 license", ["windows-server-2025"]), ("office home and business 2024", ["office-2024"]),
    ("kaspersky small office security 10 pc", ["kaspersky-small-office-security"]), ("autocad license 1 year", ["autodesk-autocad"]),
    ("kes или dr web", ["kaspersky-endpoint-security", "drweb-enterprise-security-suite"]),
    ("veeam или кибер бэкап", ["veeam-backup", "cyber-backup"]), ("мойофис или р7", ["myoffice-standard", "r7-office-professional"]),
    ("office для школы", ["cat:office-suite"]), ("антивирус для учебного заведения", ["cat:antivirus-business"]),
    ("1с для нко", ["cat:1c-configs", "cat:1c-licenses"]), ("компас для студентов", ["kompas-3d"]),
    ("windows server и 25 cal", ["windows-server-2025", "windows-server-2022", "windows-server-cal-2025", "windows-server-cal"]),
    ("рабочее место бухгалтера", ["1c-accounting", "cat:laptops", "cat:office-suite"]),
    ("всё для удалённой работы на 10 человек", ["cat:remote-access", "cat:videoconf", "cat:laptops"]),
    ("защита от утечек информации", ["infowatch-traffic-monitor"]), ("межсетевой экран для веб приложений", ["pt-application-firewall"]),
    ("шифрование и электронная подпись", ["cryptopro-csp"]), ("антивирус с сертификатом фстэк", ["kaspersky-endpoint-security", "drweb-enterprise-security-suite", "cat:antivirus-business"]),
    ("продлить 1с итс", ["1c-its"]), ("подписка adobe на год", ["adobe-creative-cloud", "adobe-photoshop", "adobe-illustrator", "adobe-acrobat"]),
    ("продление veeam", ["veeam-backup"]), ("перейти с kis на kes", ["kaspersky-endpoint-security"]),
    ("Office 365", ["microsoft-365-business", "microsoft-365-home"]), ("Win 11 Pro OEM", ["windows-11-pro"]),
    ("MS Office 2021 Professional Plus", ["office-2021"]), ("SQL Server 2022 Enterprise 2 core", ["sql-server-2022"]),
    ("Kaspersky Endpoint Security для бизнеса Расширенный 50 узлов 1 год", ["kaspersky-endpoint-security"]),
    ("Dr.Web Enterprise Security Suite комплексная защита 25", ["drweb-enterprise-security-suite"]),
    ("Astra Linux Special Edition 1.8 Смоленск бессрочная", ["astra-linux-se"]),
    ("Модуль памяти HPE 32GB DDR4", ["hpe-smart-memory-ddr4"]), ("SSD Samsung PM893 960GB", ["samsung-pm893"]),
    ("Seagate Exos 8TB", ["seagate-exos"]), ("Eltex MES2324", ["eltex-mes2300"]), ("ThinkPad E14 Gen 5", ["lenovo-thinkpad-e14-gen5"]),
]:
    q(O2, text, want)
