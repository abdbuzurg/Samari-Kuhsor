
class Component extends DCLogic {
  state = { page:'home', product:'apple', lang:'РУ', filter:'Все', query:'', formSent:false, reducedMotion:false,
    catBatch:0, catStarted:false, catParked:false, catSel:null, mapPhase:0 };

  go(page){ this.setState({ page, formSent:false, catSel:null }); if(typeof window!=='undefined') window.scrollTo(0,0); }
  open(id){ this.setState({ page:'product', product:id, formSent:false, catSel:null }); if(typeof window!=='undefined') window.scrollTo(0,0); }

  componentDidMount(){
    if (typeof window === 'undefined') return;
    if (window.matchMedia){
      this._mq = window.matchMedia('(prefers-reduced-motion: reduce)');
      this.setState({ reducedMotion: this._mq.matches });
      this._mqHandler = () => this.setState({ reducedMotion: this._mq.matches });
      if (this._mq.addEventListener) this._mq.addEventListener('change', this._mqHandler);
      else if (this._mq.addListener) this._mq.addListener(this._mqHandler);
    }
  }

  componentWillUnmount(){
    if (this._beltIO) this._beltIO.disconnect();
    if (this._mapIO) this._mapIO.disconnect();
    [this._parkTimer, this._m1, this._m2, this._m3].forEach(t => { if (t) clearTimeout(t); });
    if (this._mq){
      if (this._mq.removeEventListener) this._mq.removeEventListener('change', this._mqHandler);
      else if (this._mq.removeListener) this._mq.removeListener(this._mqHandler);
    }
  }

  setBeltRef = (el) => {
    if (!el || el === this._beltEl) return;
    this._beltEl = el;
    if (this._beltIO) this._beltIO.disconnect();
    if (this._parkTimer) clearTimeout(this._parkTimer);
    this.setState({ catStarted:false, catParked:false });
    this._beltIO = new IntersectionObserver((entries) => {
      entries.forEach(e => { if (e.isIntersecting && !this.state.catStarted){ this.setState({ catStarted:true, catParked:false }); this.armPark(); } });
    }, { threshold: 0.3 });
    this._beltIO.observe(el);
  };

  armPark(){
    if (this._parkTimer) clearTimeout(this._parkTimer);
    const dur = this.state.reducedMotion ? 380 : 1500;
    this._parkTimer = setTimeout(() => this.setState({ catParked:true }), dur);
  }

  batchGo(dir, total){
    if (total <= 1) return;
    const next = (this.state.catBatch + dir + total) % total;
    this.setState({ catBatch:next, catStarted:true, catParked:false, catSel:null });
    this.armPark();
  }

  openItem(id){ this.setState({ catSel:id }); }
  closeItem(){ this.setState({ catSel:null }); }

  setMapRef = (el) => {
    if (!el || el === this._mapEl) return;
    this._mapEl = el;
    if (this._mapIO) this._mapIO.disconnect();
    [this._m1, this._m2, this._m3].forEach(t => { if (t) clearTimeout(t); });
    this.setState({ mapPhase:0 });
    this._mapIO = new IntersectionObserver((entries) => {
      entries.forEach(e => { if (e.isIntersecting && this.state.mapPhase === 0) this.startMap(); });
    }, { threshold: 0.3 });
    this._mapIO.observe(el);
  };

  startMap(){
    if (this.state.reducedMotion){ this.setState({ mapPhase:3 }); return; }
    this.setState({ mapPhase:0.5 });
    this._m1 = setTimeout(() => this.setState({ mapPhase:1 }), 250);   // draw border stroke (~2s)
    this._m2 = setTimeout(() => this.setState({ mapPhase:2 }), 2450);  // fill fades in
    this._m3 = setTimeout(() => this.setState({ mapPhase:3 }), 3450);  // heart pops
  }

  data(){
    return [
      { id:'apple', idx:'01', line:'Соки', accent:'#6FAE3E', tint:'#E8F1DA', short:'Яблочный сок',
        name:'Яблочный сок прямого отжима', volume:'1 000 мл', pack:'Стеклянная бутылка', sku:'APJ-1000',
        desc:'Сок прямого отжима в прозрачной стеклянной бутылке 1 000 мл. Финальные заявления публикуются после утверждения рецептуры.' },
      { id:'apricot', idx:'02', line:'Джемы', accent:'#E79A3A', tint:'#FBEACB', short:'Абрикосовый джем',
        name:'Абрикосовый джем', volume:'212–228 мл', pack:'Стеклянная банка', sku:'APR-220',
        desc:'Абрикосовый джем в прозрачной стеклянной банке. Итоговый вес нетто указывается после испытаний фасовки.' },
      { id:'tomato', idx:'03', line:'Паста', accent:'#D6533B', tint:'#F7DCD3', short:'Томатная паста',
        name:'Томатная паста', volume:'500 мл', pack:'Стеклянная банка', sku:'TOM-500',
        desc:'Томатная паста в стеклянной банке. Концентрация сухих веществ и вес нетто публикуются после утверждения рецептуры.' },
      { id:'water', idx:'04', line:'Вода', accent:'#3FA3C4', tint:'#DAEDF4', short:'Питьевая вода 0,5 л',
        name:'Негазированная питьевая вода 0,5 л', volume:'500 мл', pack:'ПЭТ-бутылка', sku:'WAT-500',
        desc:'Негазированная питьевая вода в ПЭТ-бутылке 500 мл. Классификация воды не заявляется без документального подтверждения.' },
      { id:'water1l', idx:'05', line:'Вода', accent:'#3FA3C4', tint:'#DAEDF4', short:'Питьевая вода 1 л',
        name:'Негазированная питьевая вода 1 л', volume:'1 000 мл', pack:'ПЭТ-бутылка', sku:'WAT-1000',
        desc:'Негазированная питьевая вода в ПЭТ-бутылке 1 000 мл. Классификация воды не заявляется без документального подтверждения.' },
    ];
  }

  specsFor(p){
    return [
      { k:'Артикул / SKU', v:p.sku },
      { k:'Объём / нетто', v:p.volume },
      { k:'Упаковка', v:p.pack },
      { k:'Материал упаковки', v:(p.line==='Вода'?'ПЭТ':'Стекло') },
      { k:'Тип укупорки', v:(p.line==='Вода'?'Винтовая пробка':(p.id==='apple'?'Металлическая крышка':'Крышка Twist-off')) },
      { k:'Состав', v:'Уточняется после утверждения рецептуры' },
      { k:'Пищевая ценность', v:'Уточняется после лабораторного контроля' },
      { k:'Условия хранения', v:'В сухом прохладном месте, вдали от света' },
      { k:'Срок годности', v:'Уточняется после испытаний' },
      { k:'После вскрытия', v:'Хранить в холодильнике' },
      { k:'Адрес производства', v:'Тем, Хорог, ГБАО, Республика Таджикистан' },
      { k:'Статус сертификации', v:'На согласовании' },
    ].map((r,i)=>({ ...r, bg: i%2 ? '#F5F7EE' : '#ffffff' }));
  }

  renderVals(){
    const st = this.state;
    const reduced = st.reducedMotion;
    const products = this.data().map(p => ({ ...p, on: () => this.open(p.id) }));
    const p0 = products.find(x=>x.id===st.product) || products[0];
    const p = { ...p0, specs: this.specsFor(p0) };

    // ---- Assembly line (park-in-slot) ----
    const BATCH = 4;
    const totalBatches = Math.max(1, Math.ceil(products.length / BATCH));
    const multiBatch = totalBatches > 1;
    const batchProducts = products.slice(st.catBatch*BATCH, st.catBatch*BATCH+BATCH);
    const beltItems = batchProducts.map((pr,i) => {
      let animCss;
      if (!st.catStarted) animCss = 'opacity:0';
      else if (reduced) animCss = `animation:skcFade .5s ease ${(i*0.1).toFixed(2)}s backwards`;
      else animCss = `animation:skcRoll .95s cubic-bezier(.34,0,.24,1) ${(i*0.15).toFixed(2)}s backwards`;
      return { ...pr, animCss, onOpen: () => this.openItem(pr.id) };
    });
    const catalogLabel = String(st.catBatch+1).padStart(2,'0') + ' / ' + String(totalBatches).padStart(2,'0');
    const catalogBtnStyle = 'width:30px;height:24px;border-radius:6px;border:1px solid rgba(255,255,255,.22);background:rgba(255,255,255,.08);color:#EAF1DD;font-size:11px;font-family:inherit;' + (multiBatch ? 'cursor:pointer;opacity:1' : 'cursor:not-allowed;opacity:.32');
    const bstyle = this.props.beltStyle || 'green';
    const beltPos = 'position:absolute;left:44px;right:44px;top:50%;transform:translateY(-50%);height:100px;border-radius:14px;overflow:hidden;';
    let beltBg='', beltAnim='', beltShadow='box-shadow:inset 0 9px 18px rgba(0,0,0,.26),inset 0 -9px 18px rgba(0,0,0,.22),inset 0 0 0 1px rgba(0,0,0,.12);', beltClass='';
    if (bstyle==='green'){
      beltBg = 'background:linear-gradient(180deg,#4E8F63,#3E7A52 46%,#356B47 72%,#2C5A3C);';
      beltAnim = '';
    } else if (bstyle==='steel'){
      beltBg = 'background:linear-gradient(180deg,#eef1f2,#c9d1d4 20%,#b1bbc0 50%,#c9d1d4 80%,#99a3a8),repeating-linear-gradient(90deg,rgba(255,255,255,.22) 0 1px,rgba(0,0,0,.05) 1px 4px);background-size:100% 100%,8px 100%;';
      beltAnim = reduced?'':'animation:skcBelt8 .8s linear infinite;';
    } else if (bstyle==='slats'){
      beltBg = 'background:repeating-linear-gradient(90deg,#474f54 0 30px,#333a3e 30px 32px),linear-gradient(180deg,rgba(255,255,255,.15),transparent 42%,rgba(0,0,0,.34));background-size:32px 100%,100% 100%;';
      beltAnim = reduced?'':'animation:skcBelt32 1.5s linear infinite;';
    } else if (bstyle==='stone'){
      beltBg = 'background:linear-gradient(180deg,#d0d6d3,#a9b1ac),repeating-linear-gradient(118deg,rgba(255,255,255,.16) 0 20px,rgba(0,0,0,.055) 20px 40px);background-size:100% 100%,44px 100%;';
      beltAnim = reduced?'':'animation:skcBelt44 2.8s linear infinite;';
    } else if (bstyle==='minimal'){
      beltBg = 'background:linear-gradient(180deg,#eef1f1,#d7dddc 50%,#c4cbca);';
      beltAnim = '';
    } else {
      beltClass = 'skc-belt-tex';
      beltAnim = reduced?'':'animation:skcBeltTex 2.4s linear infinite;';
    }
    const beltSurfaceStyle = beltPos + beltShadow + beltBg + beltAnim;

    const sel = st.catSel ? products.find(x=>x.id===st.catSel) : null;
    const catalogModal = sel ? { ...sel, specs:this.specsFor(sel), onLearnMore: () => { this.setState({ catSel:null }); this.open(sel.id); } } : null;

    // ---- Map animation phases (draw border → fill → heart) ----
    const ph = st.mapPhase;
    const mapStrokeStyle = `fill:none;stroke:#2E7A4B;stroke-width:2.2;stroke-linejoin:round;stroke-linecap:round;stroke-dasharray:1;stroke-dashoffset:${ph>=1?0:1};transition:stroke-dashoffset ${reduced?'0s':'2s'} ease-in-out`;
    const mapFillStyle = `display:block;width:100%;height:auto;opacity:${ph>=2?1:0};transition:opacity ${reduced?'0s':'.8s'} ease`;
    const mapHeartStyle = `position:absolute;left:54.37%;top:75.56%;width:6.76%;height:auto;opacity:${ph>=3?1:0};transform:translateY(${ph>=3?'0':'-18px'}) scale(${ph>=3?1:0.72});transform-origin:50% 100%;transition:opacity .45s ease,transform .65s cubic-bezier(.34,1.45,.5,1);pointer-events:none`;
    const mapLabelStyle = `position:absolute;left:57.7%;top:63%;transform:translateX(-50%) translateY(${ph>=3?'0':'6px'});opacity:${ph>=3?1:0};transition:opacity .5s ease .15s,transform .5s ease .15s;pointer-events:none`;

    // ---- Retailer wordmarks (illustrative placeholders) ----
    const retBase = [
      { name:'ОРИЁН', color:'#23583A' }, { name:'ФАРОВОН', color:'#2E7A4B' },
      { name:'ПАЙКАР', color:'#23583A' }, { name:'САДБАРГ', color:'#E79A3A' },
      { name:'ЗАМИН', color:'#23583A' }, { name:'НАВРӮЗ', color:'#2E7A4B' },
      { name:'БАҲОР', color:'#23583A' }, { name:'ОСИЁ МАРТ', color:'#E79A3A' },
    ];
    const retailers = retBase.concat(retBase);

    // ---- Eco badges: custom line-art icons, each symbolising its claim ----
    const EG='#2E7A4B', EGD='#23583A', EA='#E79A3A';
    const P = (d, opts={}) => React.createElement('path', { d, fill:opts.fill||'none', stroke:opts.stroke===null?undefined:(opts.stroke||EG), strokeWidth:opts.sw||2.2, strokeLinecap:'round', strokeLinejoin:'round', key:opts.key });
    const ecoIcons = {
      // No preservatives → a fresh leaf (nothing artificial added)
      leaf: (tint) => [
        React.createElement('circle', { key:'t', cx:30, cy:30, r:19, fill:tint }),
        P('M30 13 C 19 21, 18 35, 30 47 C 42 35, 41 21, 30 13 Z', { key:'l' }),
        P('M30 18 L30 44', { key:'r', sw:1.9 }),
        P('M30 27 L23.5 23', { key:'v1', sw:1.7 }), P('M30 27 L36.5 23', { key:'v2', sw:1.7 }),
        P('M30 35 L22.5 31', { key:'v3', sw:1.7 }), P('M30 35 L37.5 31', { key:'v4', sw:1.7 }),
      ],
      // No additives → a pure droplet with an approving check (clean composition)
      drop: (tint) => [
        React.createElement('circle', { key:'t', cx:30, cy:30, r:19, fill:tint }),
        P('M30 13 C 23 25, 19 31, 19 37 A 11 11 0 0 0 41 37 C 41 31, 37 25, 30 13 Z', { key:'d' }),
        P('M25 37 l3.6 3.6 L36 33', { key:'c', stroke:EA, sw:2.4 }),
      ],
      // Eco production → Pamir peaks with a rising leaf sprout
      mtn: (tint) => [
        React.createElement('circle', { key:'t', cx:30, cy:30, r:19, fill:tint }),
        P('M14 42 L25 27 L31 35 L37 26 L46 42', { key:'m' }),
        P('M12 42 L48 42', { key:'b', sw:1.9 }),
        P('M25 27 L22 22.5 L25.5 21', { key:'s', stroke:EA, sw:1.9 }),
        React.createElement('circle', { key:'sun', cx:39, cy:20, r:3.4, fill:'none', stroke:EA, strokeWidth:1.9 }),
      ],
    };
    const ecoTints = { leaf:'rgba(62,142,95,.10)', drop:'rgba(63,163,174,.12)', mtn:'rgba(231,154,58,.13)' };
    const ecoArt = (kind) => React.createElement(React.Fragment, null,
      React.createElement('rect', { key:'bg', x:0.6, y:0.6, width:58.8, height:58.8, rx:15, fill:'#F7F1E6', stroke:'rgba(35,88,58,.14)', strokeWidth:1 }),
      ...ecoIcons[kind](ecoTints[kind]),
    );
    const ecoBadges = [
      { title:'Без консервантов', text:'Только природная основа — без искусственных консервантов.', art: ecoArt('leaf') },
      { title:'Без добавок', text:'Чистый состав без красителей и усилителей вкуса.', art: ecoArt('drop') },
      { title:'Эко-производство', text:'Бережный процесс на чистой воде и воздухе Памира.', art: ecoArt('mtn') },
    ];

    const lines = ['Все','Соки','Джемы','Паста','Вода'].map(l => ({
      l, on: () => this.setState({ filter:l }),
      bg: st.filter===l ? '#3E8E5A' : '#fff',
      color: st.filter===l ? '#fff' : '#23583A',
      border: st.filter===l ? '#3E8E5A' : 'rgba(35,88,58,.2)'
    }));
    const q = st.query.trim().toLowerCase();
    const filtered = products.filter(x => (st.filter==='Все'||x.line===st.filter) && (!q || x.name.toLowerCase().includes(q)));

    const navDefs = [
      { key:'home', label:'Главная', active: st.page==='home', on:()=>this.go('home') },
      { key:'catalogue', label:'Продукция', active: st.page==='catalogue'||st.page==='product', on:()=>this.go('catalogue') },
      { key:'production', label:'Производство и качество', active: st.page==='production', on:()=>this.go('production') },
      { key:'contact', label:'Контакты', active: st.page==='contact', on:()=>this.go('contact') },
    ].map(n => ({ ...n, color: n.active ? '#23583A':'#4E5D53', bg: n.active ? 'rgba(62,142,95,.13)':'transparent' }));

    const langs = ['ТҶ','РУ','EN'].map(l => ({
      l, on:()=>this.setState({ lang:l }),
      color: st.lang===l ? '#fff':'#23583A',
      bg: st.lang===l ? '#3E8E5A':'transparent'
    }));

    const trustItems = [
      { title:'Лабораторный контроль', text:'Проверки на этапах производства до публикации заявлений.' },
      { title:'Прослеживаемость', text:'Контроль сырья от приёмки до готовой продукции.' },
      { title:'Гигиена и санитария', text:'Протоколы очистки и HACCP-подход на площадке.' },
      { title:'Профессиональная упаковка', text:'Стекло и ПЭТ, современные форматы и укупорка.' },
    ];

    const processFull = [
      { n:'01', title:'Приёмка и контроль сырья', text:'Инспекция фруктов, овощей и воды при поступлении.' },
      { n:'02', title:'Переработка фруктов и овощей', text:'Мойка, подготовка и технологическая переработка.' },
      { n:'03', title:'Водоподготовка и выдув ПЭТ', text:'Подготовка воды и формование ПЭТ-бутылок.' },
      { n:'04', title:'Пастеризация и розлив', text:'Термическая обработка и розлив в тару.' },
      { n:'05', title:'Укупорка, охлаждение, маркировка', text:'Герметизация, охлаждение и нанесение маркировки.' },
      { n:'06', title:'Упаковка и складирование', text:'Групповая упаковка, форматы и хранение.' },
    ];
    const processPreview = processFull.slice(0,6).map(s=>({ n:s.n, title:s.title }));

    const news = [
      { cat:'Строительство', date:'Июнь 2026', title:'Строительство производственной площадки в Теме', tint:'#E8F1DA' },
      { cat:'Оборудование', date:'Июль 2026', title:'Монтаж технологического оборудования', tint:'#FBEACB' },
      { cat:'Запуск', date:'Август 2026', title:'Подготовка к запуску четырёх продуктовых линий', tint:'#DAEDF4' },
    ];

    return {
      isHome: st.page==='home', isCatalogue: st.page==='catalogue', isProduct: st.page==='product',
      isProduction: st.page==='production', isContact: st.page==='contact',
      products, filtered, lines, p,
      beltItems, catalogModal, isCatalogModalOpen: !!catalogModal, catalogLabel, catalogBtnStyle, beltSurfaceStyle, beltClass,
      setBeltRef: this.setBeltRef, setMapRef: this.setMapRef,
      catalogPrev: () => this.batchGo(-1, totalBatches),
      catalogNext: () => this.batchGo(1, totalBatches),
      closeCatalogItem: () => this.closeItem(),
      stopProp: (e) => e.stopPropagation(),
      mapStrokeStyle, mapFillStyle, mapHeartStyle, mapLabelStyle,
      retailers, ecoBadges,
      navItems: navDefs, langs, trustItems, processFull, processPreview, news,
      goCatalogue: ()=>this.go('catalogue'), goProduction: ()=>this.go('production'), goContact: ()=>this.go('contact'),
      onQuery: (e)=>this.setState({ query: e.target.value }),
      formSent: st.formSent, formOpen: !st.formSent,
      submitForm: (e)=>{ e.preventDefault(); this.setState({ formSent:true }); },
    };
  }
}
