<script setup lang="ts">
// Путь запроса (search_b3.go): нормализация → условия → маршрут; разбор LLM только на D; BM25 и векторы
// параллельно → RRF → семейства и порог → реранкер только на C и D → бизнес-слой → цель и лестница
// «минимум 4 товара» (search_ladder.go) → ответ.
import { THEORY_PARAMS as P, ru } from '@/lib/theoryParams'
import DgArrow from '../diagram/DgArrow.vue'
import DgDecision from '../diagram/DgDecision.vue'
import DgDot from '../diagram/DgDot.vue'
import DgLabel from '../diagram/DgLabel.vue'
import DgNode from '../diagram/DgNode.vue'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const top = `до ${ru(P.candidatesTop)} кандидатов`
const ladder = `добор до ${ru(P.ladderMin)} семейств`
</script>

<template>
  <TheoryDiagram
    title="Путь запроса от нормализации до ответа"
    :desc="`Нормализация, условия и названия, выбор маршрута. На маршруте D — разбор LLM, маршруты A–C идут мимо него. Затем BM25 и векторы параллельно, ${top} в каждом списке, и слияние RRF. Семейства и порог. Реранкер — только на маршрутах C и D, A и B идут мимо. Итоговое ранжирование. Целевая страница и добор выдачи: карточки получают тип совпадения, и если разных семейств меньше ${ru(P.ladderMin)}, добор выдачи подставляет подписанные карточки. Ответ: цель, выдача, чипы и трассировка.`"
    :wide="[720, 296]"
    :narrow="[328, 762]"
  >
    <template #wide>
      <DgNode :x="0" :y="63" :w="112" :h="34" title="Нормализация" />
      <DgArrow pts="112,80 134,80" />
      <DgNode :x="134" :y="63" :w="142" :h="34" title="Условия и названия" />
      <DgArrow pts="276,80 298,80" />
      <DgDecision :x="298" :y="60" :w="100" :h="40" title="Маршрут?" />

      <DgArrow pts="398,80 470,80 470,50 486,50" label="A–C" :lx="424" :ly="71" />
      <DgArrow pts="470,80 470,110 486,110" />
      <DgDot :x="470" :y="80" />
      <DgArrow pts="348,100 348,140" tone="d" label="D" :lx="356" :ly="120" anchor="start" />
      <DgNode
        :x="275"
        :y="140"
        :w="146"
        :h="50"
        kind="cond"
        tone="d"
        title="Разбор LLM"
        sub="кэш, затем модель"
      />
      <DgArrow pts="421,165 446,165 446,84" tone="d" />

      <DgLabel :x="529" :y="10" :text="top" tone="muted" />
      <DgNode :x="486" :y="25" :w="86" :h="50" title="BM25" sub="по словам" />
      <DgNode :x="486" :y="85" :w="86" :h="50" title="Векторы" sub="по смыслу" />
      <DgArrow pts="572,50 586,50 586,80 604,80" />
      <DgArrow pts="572,110 586,110 586,80" :head="false" />
      <DgDot :x="586" :y="80" />
      <DgNode :x="604" :y="63" :w="116" :h="34" title="Слияние RRF" />

      <DgArrow pts="662,97 662,241" />
      <DgNode :x="604" :y="241" :w="116" :h="50" title="Семейства" sub="порог, условия" />
      <DgArrow pts="604,266 572,266" />
      <DgArrow
        pts="588,266 588,218 400,218 400,241"
        kind="skip"
        label="A и B идут мимо"
        :lx="494"
        :ly="207"
      />
      <DgDot :x="588" :y="266" />
      <DgNode :x="462" :y="241" :w="110" :h="50" kind="cond" title="Реранкер" sub="только C и D" />
      <DgArrow pts="462,266 424,266" />
      <DgNode :x="306" :y="241" :w="118" :h="50" :title="['Итоговое', 'ранжирование']" />
      <DgArrow pts="306,266 284,266" />
      <DgNode :x="146" :y="241" :w="138" :h="50" title="Целевая страница" :sub="ladder" />
      <DgArrow pts="146,266 124,266" />
      <DgNode :x="0" :y="241" :w="124" :h="50" kind="term" title="Ответ" sub="цель и выдача" />
    </template>

    <template #narrow>
      <DgNode :x="64" :y="0" :w="200" :h="32" title="Нормализация" />
      <DgArrow pts="164,32 164,54" />
      <DgNode :x="64" :y="54" :w="200" :h="32" title="Условия и названия" />
      <DgArrow pts="164,86 164,108" />
      <DgDecision :x="104" :y="108" :w="120" :h="36" title="Маршрут?" />
      <DgArrow pts="164,144 164,170" tone="d" label="D" :lx="172" :ly="157" anchor="start" />
      <DgNode
        :x="74"
        :y="170"
        :w="180"
        :h="46"
        kind="cond"
        tone="d"
        title="Разбор LLM"
        sub="кэш, затем модель"
      />
      <DgArrow
        pts="104,126 24,126 24,238 160,238"
        kind="skip"
        label="A–C"
        :lx="32"
        :ly="190"
        anchor="start"
      />
      <DgArrow pts="164,216 164,252 96,252 96,266" />
      <DgArrow pts="164,252 232,252 232,266" />
      <DgDot :x="164" :y="252" />
      <DgNode :x="36" :y="266" :w="120" :h="62" title="BM25" :sub="['по словам', top]" />
      <DgNode :x="172" :y="266" :w="120" :h="62" title="Векторы" :sub="['по смыслу', top]" />
      <DgArrow pts="96,328 96,342 164,342 164,360" />
      <DgArrow pts="232,328 232,342 164,342" :head="false" />
      <DgDot :x="164" :y="342" />
      <DgNode :x="64" :y="360" :w="200" :h="32" title="Слияние RRF" />
      <DgArrow pts="164,392 164,414" />
      <DgNode :x="64" :y="414" :w="200" :h="46" title="Семейства" sub="порог, условия" />
      <DgArrow pts="164,460 164,492" />
      <DgArrow
        pts="164,472 24,472 24,601 64,601"
        kind="skip"
        label="A и B"
        :lx="32"
        :ly="536"
        anchor="start"
      />
      <DgDot :x="164" :y="472" />
      <DgNode :x="74" :y="492" :w="180" :h="46" kind="cond" title="Реранкер" sub="только C и D" />
      <DgArrow pts="164,538 164,578" />
      <DgNode :x="64" :y="578" :w="200" :h="46" title="Итоговое ранжирование" sub="группы, скор" />
      <DgArrow pts="164,624 164,646" />
      <DgNode :x="64" :y="646" :w="200" :h="46" title="Целевая страница" :sub="ladder" />
      <DgArrow pts="164,692 164,714" />
      <DgNode
        :x="64"
        :y="714"
        :w="200"
        :h="46"
        kind="term"
        title="Ответ"
        sub="цель, выдача, чипы, трассировка"
      />
    </template>

    <template #caption>
      Сплошные стрелки — путь, который проходит каждый запрос. Пунктирная рамка — шаг только для
      части маршрутов, пунктирная стрелка — обход этого шага остальными.
    </template>
  </TheoryDiagram>
</template>
